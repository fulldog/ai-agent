package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/fileextract"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
)

type ChatHandler struct {
	Chat        *chat.Service
	FileExtract *fileextract.Service
}

func (h *ChatHandler) parseInput(c *gin.Context) (chat.CompleteInput, bool) {
	var req struct {
		ConversationID string  `json:"conversation_id" binding:"required"`
		Message        string  `json:"message" binding:"required"`
		Provider       string  `json:"provider"`
		Model          string  `json:"model"`
		Temperature    float64 `json:"temperature"`
		MaxTokens      int     `json:"max_tokens"`
		RAG            *struct {
			Enabled  bool   `json:"enabled"`
			CorpusID string `json:"corpus_id"`
			TopK     int    `json:"top_k"`
		} `json:"rag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return chat.CompleteInput{}, false
	}
	cid, err := uuid.Parse(req.ConversationID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid conversation_id")
		return chat.CompleteInput{}, false
	}
	in := chat.CompleteInput{
		ConversationID: cid,
		Message:        req.Message,
		Provider:       req.Provider,
		Model:          req.Model,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		RequestID:      requestID(c),
	}
	if req.RAG != nil {
		in.RAGEnabled = req.RAG.Enabled
		in.TopK = req.RAG.TopK
		if req.RAG.CorpusID != "" {
			id, err := uuid.Parse(req.RAG.CorpusID)
			if err != nil {
				writeError(c, http.StatusBadRequest, "bad_request", "invalid corpus_id")
				return chat.CompleteInput{}, false
			}
			in.CorpusID = &id
		}
	}
	return in, true
}

// Completions POST /api/v1/chat/completions
func (h *ChatHandler) Completions(c *gin.Context) {
	in, ok := h.parseInput(c)
	if !ok {
		return
	}
	res, err := h.Chat.Complete(c.Request.Context(), in)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"conversation_id": res.ConversationID,
		"message_id":      res.MessageID,
		"content":         res.Content,
		"usage": gin.H{
			"prompt_tokens":     res.PromptTokens,
			"completion_tokens": res.CompletionTokens,
			"total_tokens":      res.PromptTokens + res.CompletionTokens,
		},
	})
}

// CompletionsStream POST /api/v1/chat/completions/stream
func (h *ChatHandler) CompletionsStream(c *gin.Context) {
	in, ok := h.parseInput(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	res, err := h.Chat.CompleteStream(c.Request.Context(), in, func(delta string) error {
		return writeSSE(c, "delta", gin.H{"content": delta})
	})
	if err != nil {
		_ = writeSSE(c, "error", gin.H{"message": err.Error()})
		return
	}
	_ = writeSSE(c, "done", gin.H{
		"conversation_id": res.ConversationID,
		"message_id":      res.MessageID,
		"content":         res.Content,
		"usage": gin.H{
			"prompt_tokens":     res.PromptTokens,
			"completion_tokens": res.CompletionTokens,
			"total_tokens":      res.PromptTokens + res.CompletionTokens,
		},
	})
}

func parseFieldsForm(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			return trimFields(arr)
		}
	}
	return trimFields(strings.Split(raw, ","))
}

func trimFields(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseBoolForm(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (h *ChatHandler) parseAnalyze(c *gin.Context) (chat.AnalyzeInput, bool) {
	ct := c.ContentType()
	in := chat.AnalyzeInput{RequestID: requestID(c)}

	if strings.HasPrefix(ct, "multipart/form-data") {
		in.Message = strings.TrimSpace(c.PostForm("message"))
		in.Fields = parseFieldsForm(c.PostForm("fields"))
		if in.Message == "" && len(in.Fields) == 0 {
			writeError(c, http.StatusBadRequest, "bad_request", "message 或 fields 至少填一个")
			return chat.AnalyzeInput{}, false
		}
		in.Provider = c.PostForm("provider")
		in.Model = c.PostForm("model")
		rf := strings.TrimSpace(strings.ToLower(c.PostForm("response_format")))
		if rf == "json" || rf == "json_object" || len(in.Fields) > 0 {
			in.ResponseJSON = true
		}
		if v := c.PostForm("temperature"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				in.Temperature = f
				in.TempSet = true
			}
		}
		if v := c.PostForm("max_tokens"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				in.MaxTokens = n
			}
		}
		if cid := strings.TrimSpace(c.PostForm("conversation_id")); cid != "" {
			id, err := uuid.Parse(cid)
			if err != nil {
				writeError(c, http.StatusBadRequest, "bad_request", "invalid conversation_id")
				return chat.AnalyzeInput{}, false
			}
			in.ConversationID = &id
		}
		force := parseBoolForm(c.PostForm("force_reread"))

		file, err := c.FormFile("file")
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", "file required")
			return chat.AnalyzeInput{}, false
		}
		if !extract.IsSupportedExtension(file.Filename) {
			writeError(c, http.StatusBadRequest, "bad_request", "unsupported file type (txt/md/pdf/docx/png/jpg/jpeg/webp/bmp/tif/gif)")
			return chat.AnalyzeInput{}, false
		}
		f, err := file.Open()
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return chat.AnalyzeInput{}, false
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return chat.AnalyzeInput{}, false
		}
		if h.FileExtract == nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "fileextract not configured")
			return chat.AnalyzeInput{}, false
		}
		resolved, err := h.FileExtract.Prepare(c.Request.Context(), fileextract.PrepareInput{
			Filename: file.Filename, Data: b, Force: force, Provider: in.Provider,
		})
		if err != nil {
			if errors.Is(err, fileextract.ErrBusy) {
				writeError(c, http.StatusConflict, "busy", err.Error())
				return chat.AnalyzeInput{}, false
			}
			writeError(c, http.StatusBadRequest, "bad_request", "extract failed: "+err.Error())
			return chat.AnalyzeInput{}, false
		}
		in.FileName = file.Filename
		in.FileText = resolved.Text
		in.RemoteFileID = resolved.RemoteFileID
		in.FileMode = resolved.Mode
		in.ContentHash = resolved.ContentHash
		in.CacheHit = resolved.CacheHit
		id := resolved.ExtractionID
		in.ExtractionID = &id
		in.ExtractBackend = resolved.ExtractBackend
		in.ChatModelHint = resolved.ChatModelHint
		return in, true
	}

	var req struct {
		ConversationID string   `json:"conversation_id"`
		Message        string   `json:"message"`
		Content        string   `json:"content" binding:"required"`
		FileName       string   `json:"file_name"`
		Fields         []string `json:"fields"`
		ResponseFormat string   `json:"response_format"` // json | text
		Provider       string   `json:"provider"`
		Model          string   `json:"model"`
		Temperature    *float64 `json:"temperature"`
		MaxTokens      int      `json:"max_tokens"`
		ForceReread    bool     `json:"force_reread"` // JSON 直传正文时忽略
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return chat.AnalyzeInput{}, false
	}
	_ = req.ForceReread
	in.Message = req.Message
	in.Fields = trimFields(req.Fields)
	in.FileText = req.Content
	in.FileName = req.FileName
	in.Provider = req.Provider
	in.Model = req.Model
	in.MaxTokens = req.MaxTokens
	rf := strings.ToLower(strings.TrimSpace(req.ResponseFormat))
	if rf == "json" || rf == "json_object" || len(in.Fields) > 0 {
		in.ResponseJSON = true
	}
	if req.Temperature != nil {
		in.Temperature = *req.Temperature
		in.TempSet = true
	}
	if req.Message == "" && len(in.Fields) == 0 {
		writeError(c, http.StatusBadRequest, "bad_request", "message 或 fields 至少填一个")
		return chat.AnalyzeInput{}, false
	}
	if req.ConversationID != "" {
		id, err := uuid.Parse(req.ConversationID)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", "invalid conversation_id")
			return chat.AnalyzeInput{}, false
		}
		in.ConversationID = &id
	}
	return in, true
}

func analyzeResponse(res *chat.AnalyzeResult) gin.H {
	out := gin.H{
		"file_name":  res.FileName,
		"file_chars": res.FileChars,
		"truncated":  res.Truncated,
		"cache_hit":  res.CacheHit,
		"usage": gin.H{
			"prompt_tokens":     res.PromptTokens,
			"completion_tokens": res.CompletionTokens,
			"total_tokens":      res.PromptTokens + res.CompletionTokens,
		},
	}
	if res.ContentHash != "" {
		out["content_hash"] = res.ContentHash
	}
	if res.ExtractionID != nil {
		out["extraction_id"] = res.ExtractionID
	}
	if res.ExtractBackend != "" {
		out["extract_backend"] = res.ExtractBackend
	}
	if res.Data != nil {
		out["data"] = res.Data
	}
	out["content"] = res.Content
	if res.ConversationID != nil {
		out["conversation_id"] = res.ConversationID
	}
	if res.MessageID != nil {
		out["message_id"] = res.MessageID
	}
	return out
}

// Analyze POST /api/v1/chat/analyze — 上传文件直接给大模型分析（不进语料库）。
func (h *ChatHandler) Analyze(c *gin.Context) {
	in, ok := h.parseAnalyze(c)
	if !ok {
		return
	}
	res, err := h.Chat.Analyze(c.Request.Context(), in, nil)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	c.JSON(http.StatusOK, analyzeResponse(res))
}

// AnalyzeStream POST /api/v1/chat/analyze/stream
func (h *ChatHandler) AnalyzeStream(c *gin.Context) {
	in, ok := h.parseAnalyze(c)
	if !ok {
		return
	}
	in.Stream = true
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	res, err := h.Chat.Analyze(c.Request.Context(), in, func(delta string) error {
		return writeSSE(c, "delta", gin.H{"content": delta})
	})
	if err != nil {
		_ = writeSSE(c, "error", gin.H{"message": err.Error()})
		return
	}
	_ = writeSSE(c, "done", analyzeResponse(res))
}
