package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/fileextract"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
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

func (h *CorpusHandler) AddDocument(c *gin.Context) {
	corpusID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	ct := c.ContentType()
	var title, source, content string
	var cacheHit bool
	var contentHash string
	var extractionID *uuid.UUID

	if len(ct) >= 19 && ct[:19] == "multipart/form-data" {
		file, err := c.FormFile("file")
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", "file required")
			return
		}
		if !extract.IsSupportedExtension(file.Filename) {
			writeError(c, http.StatusBadRequest, "bad_request", "unsupported file type (txt/md/pdf/docx/png/jpg/jpeg/webp/bmp/tif/gif)")
			return
		}
		f, err := file.Open()
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		title = extract.GuessName(file.Filename)
		source = file.Filename
		if h.FileExtract == nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "fileextract not configured")
			return
		}
		force := parseBoolForm(c.PostForm("force_reread"))
		resolved, err := h.FileExtract.Resolve(c.Request.Context(), file.Filename, b, force)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		content = resolved.Text
		cacheHit = resolved.CacheHit
		contentHash = resolved.ContentHash
		id := resolved.ExtractionID
		extractionID = &id
	} else {
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
		title, content = req.Title, req.Content
		source = req.Title
	}
	doc, err := h.Corpus.AddDocument(c.Request.Context(), corpus.AddDocumentInput{
		CorpusID: corpusID, Title: title, Source: source, Content: content,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := gin.H{"document": doc, "cache_hit": cacheHit}
	if contentHash != "" {
		out["content_hash"] = contentHash
	}
	if extractionID != nil {
		out["extraction_id"] = extractionID
	}
	c.JSON(http.StatusCreated, out)
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
