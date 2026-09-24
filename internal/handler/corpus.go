package handler

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/fileextract"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
	"gorm.io/gorm"
)

type CorpusHandler struct {
	Corpus      *corpus.Service
	FileExtract *fileextract.Service
}

func (h *CorpusHandler) Create(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	row, err := h.Corpus.Create(req.Name, req.Description)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *CorpusHandler) List(c *gin.Context) {
	rows, err := h.Corpus.List()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *CorpusHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	row, err := h.Corpus.Get(id)
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "corpus not found")
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *CorpusHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Corpus.Delete(id); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CorpusHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		Name        string  `json:"name" binding:"required"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	desc := ""
	setDesc := false
	if req.Description != nil {
		desc = *req.Description
		setDesc = true
	}
	row, err := h.Corpus.Update(id, req.Name, desc, setDesc)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "record not found") {
			writeError(c, http.StatusNotFound, "not_found", "corpus not found")
			return
		}
		if strings.Contains(err.Error(), "name required") {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		// unique violation
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(err.Error(), "unique") {
			writeError(c, http.StatusConflict, "conflict", "语料库名称已存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *CorpusHandler) AddDocument(c *gin.Context) {
	corpusID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	ct := c.ContentType()

	if strings.HasPrefix(ct, "multipart/form-data") {
		h.addDocumentsMultipart(c, corpusID)
		return
	}

	var req struct {
		Title       string `json:"title"`
		Content     string `json:"content" binding:"required"`
		ForceReread bool   `json:"force_reread"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	_ = req.ForceReread
	doc, err := h.Corpus.AddDocument(c.Request.Context(), corpus.AddDocumentInput{
		CorpusID: corpusID, Title: req.Title, Source: req.Title, Content: req.Content,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"document": doc, "cache_hit": false})
}

type uploadItemResult struct {
	Filename       string     `json:"filename"`
	Document       any        `json:"document,omitempty"`
	CacheHit       bool       `json:"cache_hit,omitempty"`
	ContentHash    string     `json:"content_hash,omitempty"`
	ExtractionID   *uuid.UUID `json:"extraction_id,omitempty"`
	ExtractBackend string     `json:"extract_backend,omitempty"`
	Error          string     `json:"error,omitempty"`
}

func (h *CorpusHandler) addDocumentsMultipart(c *gin.Context, corpusID uuid.UUID) {
	form, err := c.MultipartForm()
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid multipart: "+err.Error())
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	if len(files) == 0 {
		// 兼容单文件 FormFile
		if f, ferr := c.FormFile("file"); ferr == nil && f != nil {
			files = []*multipart.FileHeader{f}
		}
	}
	if len(files) == 0 {
		writeError(c, http.StatusBadRequest, "bad_request", "file or files required")
		return
	}
	force := parseBoolForm(c.PostForm("force_reread"))
	provider := strings.TrimSpace(c.PostForm("provider"))

	items := make([]uploadItemResult, 0, len(files))
	ok, failed := 0, 0
	for _, fh := range files {
		item := h.ingestUploadedFile(c, corpusID, fh, force, provider)
		items = append(items, item)
		if item.Error != "" {
			failed++
		} else {
			ok++
		}
	}

	if len(items) == 1 {
		it := items[0]
		if it.Error != "" {
			status := http.StatusBadRequest
			code := "bad_request"
			if strings.Contains(it.Error, "busy") || strings.Contains(it.Error, "繁忙") {
				status = http.StatusConflict
				code = "busy"
			}
			writeError(c, status, code, it.Error)
			return
		}
		out := gin.H{"document": it.Document, "cache_hit": it.CacheHit}
		if it.ContentHash != "" {
			out["content_hash"] = it.ContentHash
		}
		if it.ExtractionID != nil {
			out["extraction_id"] = it.ExtractionID
		}
		if it.ExtractBackend != "" {
			out["extract_backend"] = it.ExtractBackend
		}
		c.JSON(http.StatusCreated, out)
		return
	}

	status := http.StatusCreated
	if ok == 0 {
		status = http.StatusBadRequest
	} else if failed > 0 {
		status = http.StatusMultiStatus // 207
	}
	c.JSON(status, gin.H{"items": items, "ok": ok, "failed": failed})
}

func (h *CorpusHandler) ingestUploadedFile(c *gin.Context, corpusID uuid.UUID, file *multipart.FileHeader, force bool, provider string) uploadItemResult {
	out := uploadItemResult{Filename: file.Filename}
	if !extract.IsSupportedExtension(file.Filename) {
		out.Error = "unsupported file type (txt/md/pdf/docx/png/jpg/jpeg/webp/bmp/tif/gif)"
		return out
	}
	f, err := file.Open()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if h.FileExtract == nil {
		out.Error = "fileextract not configured"
		return out
	}
	resolved, err := h.FileExtract.Prepare(c.Request.Context(), fileextract.PrepareInput{
		Filename: file.Filename, Data: b, Force: force, Provider: provider, NeedText: true,
	})
	if err != nil {
		out.Error = err.Error()
		return out
	}
	doc, err := h.Corpus.AddDocument(c.Request.Context(), corpus.AddDocumentInput{
		CorpusID: corpusID,
		Title:    extract.GuessName(file.Filename),
		Source:   file.Filename,
		Content:  resolved.Text,
	})
	if err != nil {
		out.Error = err.Error()
		if doc != nil {
			out.Document = doc
		}
		return out
	}
	out.Document = doc
	out.CacheHit = resolved.CacheHit
	out.ContentHash = resolved.ContentHash
	id := resolved.ExtractionID
	out.ExtractionID = &id
	out.ExtractBackend = resolved.ExtractBackend
	return out
}

func (h *CorpusHandler) ListDocuments(c *gin.Context) {
	corpusID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	rows, err := h.Corpus.ListDocuments(corpusID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *CorpusHandler) DeleteDocument(c *gin.Context) {
	corpusID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	docID, err := uuid.Parse(c.Param("doc_id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid doc_id")
		return
	}
	if err := h.Corpus.DeleteDocument(corpusID, docID); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CorpusHandler) Reindex(c *gin.Context) {
	corpusID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Corpus.Reindex(c.Request.Context(), corpusID); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
