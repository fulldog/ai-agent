package handler

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
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
		CorpusID: corpusID, Title: req.Title, Source: req.Title, Content: req.Content, Kind: "text",
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

	// 每个文件单独建文档并索引，正文互不拼接。
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
	prepared, err := h.prepareUpload(c, file, force, provider)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	doc, err := h.Corpus.AddDocument(c.Request.Context(), corpus.AddDocumentInput{
		CorpusID:     corpusID,
		Title:        prepared.Title,
		Source:       prepared.Source,
		Content:      prepared.Text,
		Kind:         "file",
		ExtractionID: extractionPtr(prepared.ExtractionID),
	})
	if err != nil {
		out.Error = err.Error()
		if doc != nil {
			out.Document = doc
		}
		return out
	}
	return prepared.item(doc)
}

type preparedUpload struct {
	Filename       string
	Title          string
	Source         string
	Text           string
	CacheHit       bool
	ContentHash    string
	ExtractionID   uuid.UUID
	ExtractBackend string
}

func (p preparedUpload) item(doc any) uploadItemResult {
	id := p.ExtractionID
	return uploadItemResult{
		Filename:       p.Filename,
		Document:       doc,
		CacheHit:       p.CacheHit,
		ContentHash:    p.ContentHash,
		ExtractionID:   &id,
		ExtractBackend: p.ExtractBackend,
	}
}

func (h *CorpusHandler) prepareUpload(c *gin.Context, file *multipart.FileHeader, force bool, provider string) (preparedUpload, error) {
	out := preparedUpload{
		Filename: file.Filename,
		Title:    extract.GuessName(file.Filename),
		Source:   file.Filename,
	}
	if !extract.IsSupportedExtension(file.Filename) {
		return out, errors.New("unsupported file type (txt/md/pdf/docx/png/jpg/jpeg/webp/bmp/tif/gif)")
	}
	f, err := file.Open()
	if err != nil {
		return out, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return out, err
	}
	if h.FileExtract == nil {
		return out, errors.New("fileextract not configured")
	}
	resolved, err := h.FileExtract.Prepare(c.Request.Context(), fileextract.PrepareInput{
		Filename: file.Filename, Data: b, Force: force, Provider: provider, NeedText: true,
	})
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(resolved.Text) == "" {
		return out, errors.New("未能从文件中提取到正文")
	}
	out.Text = resolved.Text
	out.CacheHit = resolved.CacheHit
	out.ContentHash = resolved.ContentHash
	out.ExtractionID = resolved.ExtractionID
	out.ExtractBackend = resolved.ExtractBackend
	return out, nil
}

func (h *CorpusHandler) ReuploadDocument(c *gin.Context) {
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
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		writeError(c, http.StatusBadRequest, "bad_request", "file required")
		return
	}
	prepared, err := h.prepareUpload(
		c,
		file,
		parseBoolForm(c.PostForm("force_reread")),
		strings.TrimSpace(c.PostForm("provider")),
	)
	if err != nil {
		status := http.StatusBadRequest
		code := "bad_request"
		if strings.Contains(err.Error(), "busy") || strings.Contains(err.Error(), "繁忙") {
			status = http.StatusConflict
			code = "busy"
		}
		writeError(c, status, code, err.Error())
		return
	}
	doc, err := h.Corpus.ReplaceDocument(c.Request.Context(), corpusID, docID, corpus.AddDocumentInput{
		CorpusID:     corpusID,
		Title:        prepared.Title,
		Source:       prepared.Source,
		Content:      prepared.Text,
		Kind:         "file",
		ExtractionID: extractionPtr(prepared.ExtractionID),
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "record not found") {
			writeError(c, http.StatusNotFound, "not_found", "document not found")
			return
		}
		if doc != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"document": doc, "error": err.Error()})
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, prepared.item(doc))
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

func (h *CorpusHandler) GetDocument(c *gin.Context) {
	corpusID, docID, ok := documentIDs(c)
	if !ok {
		return
	}
	doc, err := h.Corpus.GetDocument(corpusID, docID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "document not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	content, err := h.Corpus.DocumentContent(doc)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"document": doc, "kind": doc.Kind, "content": content})
}

func (h *CorpusHandler) DownloadDocument(c *gin.Context) {
	corpusID, docID, ok := documentIDs(c)
	if !ok {
		return
	}
	doc, err := h.Corpus.GetDocument(corpusID, docID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "document not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if doc.Kind != "file" {
		writeError(c, http.StatusBadRequest, "bad_request", "该文档是文本，请打开查看")
		return
	}
	if h.FileExtract != nil {
		abs, name, err := h.FileExtract.OpenOriginal(c.Request.Context(), doc.ExtractionID, doc.Source)
		if err == nil {
			if name == "" {
				name = doc.Source
			}
			sendDownload(c, name, nil, abs)
			return
		}
		if !missingOriginal(err) {
			writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	content, err := h.Corpus.DocumentContent(doc)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if strings.TrimSpace(content) == "" {
		writeError(c, http.StatusNotFound, "not_found", "原始文件不存在")
		return
	}
	sendDownload(c, extractedDownloadName(doc.Source, doc.Title), []byte(content), "")
}

func missingOriginal(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) ||
		strings.Contains(err.Error(), "原始文件不存在") ||
		strings.Contains(err.Error(), "invalid path")
}

func extractedDownloadName(source, title string) string {
	name := strings.TrimSpace(source)
	if name == "" {
		name = strings.TrimSpace(title)
	}
	if name == "" {
		name = "document.txt"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".xml", ".html", ".htm":
		return name
	default:
		base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		if base == "" {
			base = "document"
		}
		return base + ".txt"
	}
}

func sendDownload(c *gin.Context, name string, body []byte, absPath string) {
	ctype := mime.TypeByExtension(filepath.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if strings.HasPrefix(ctype, "text/") && !strings.Contains(ctype, "charset") {
		ctype += "; charset=utf-8"
	}
	c.Header("Content-Disposition", contentDisposition(name, inlinePreview(name)))
	if absPath != "" {
		c.File(absPath)
		return
	}
	c.Data(http.StatusOK, ctype, body)
}

func documentIDs(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	corpusID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return uuid.Nil, uuid.Nil, false
	}
	docID, err := uuid.Parse(c.Param("doc_id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid doc_id")
		return uuid.Nil, uuid.Nil, false
	}
	return corpusID, docID, true
}

func extractionPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func inlinePreview(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func contentDisposition(name string, inline bool) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	if name == "" {
		name = "download"
	}
	var fallback strings.Builder
	for _, r := range name {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			fallback.WriteByte('_')
			continue
		}
		fallback.WriteRune(r)
	}
	fb := fallback.String()
	if strings.Trim(fb, "_") == "" {
		fb = "download"
	}
	disp := "attachment"
	if inline {
		disp = "inline"
	}
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disp, fb, url.PathEscape(name))
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
