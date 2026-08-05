package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// FileObject OpenAI 兼容 Files 上传响应。
type FileObject struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Bytes  int64  `json:"bytes"`
	Name   string `json:"filename"`
}

// FilesClient OpenAI 兼容 /files（Kimi / 通义兼容模式）。
type FilesClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewFilesClient(baseURL, apiKey string, timeout time.Duration) *FilesClient {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &FilesClient{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *FilesClient) Upload(ctx context.Context, filename string, data []byte, purpose string) (*FileObject, error) {
	if c == nil || c.APIKey == "" {
		return nil, fmt.Errorf("files client 未配置 api_key")
	}
	if purpose == "" {
		purpose = "file-extract"
	}
	baseName := filepath.Base(filename)
	if baseName == "" || baseName == "." {
		baseName = "upload.bin"
	}
	// 部分网关对 Content-Disposition 非 ASCII 文件名解析失败，导致无 id；上传用安全名。
	uploadName := sanitizeUploadFilename(baseName)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", uploadName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := w.WriteField("purpose", purpose); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	url := c.BaseURL + "/files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upload files HTTP %d url=%s: %s", resp.StatusCode, url, truncate(string(raw), 800))
	}
	fo, err := parseFileObject(raw)
	if err != nil {
		return nil, fmt.Errorf("parse upload response: %w; body=%s", err, truncate(string(raw), 800))
	}
	if fo.ID == "" {
		return nil, fmt.Errorf("upload files: empty file id; url=%s body=%s", url, truncate(string(raw), 800))
	}
	if fo.Name == "" {
		fo.Name = baseName
	}
	return fo, nil
}

func sanitizeUploadFilename(name string) string {
	ext := filepath.Ext(name)
	for _, r := range name {
		if r > 127 {
			if ext == "" {
				return "upload.bin"
			}
			return "upload" + strings.ToLower(ext)
		}
	}
	// 去掉路径分隔等危险字符
	name = strings.ReplaceAll(name, `\`, "_")
	name = strings.ReplaceAll(name, `/`, "_")
	name = strings.ReplaceAll(name, `"`, "_")
	if strings.TrimSpace(name) == "" {
		return "upload.bin"
	}
	return name
}

func parseFileObject(raw []byte) (*FileObject, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty response body")
	}
	// 直接对象
	var fo FileObject
	if err := json.Unmarshal(raw, &fo); err == nil && fo.ID != "" {
		return &fo, nil
	}
	// 兼容 file_id / 嵌套 data|output|result
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if id := jsonString(envelope["id"]); id != "" {
		fo.ID = id
		fo.Status = jsonString(envelope["status"])
		fo.Name = jsonString(envelope["filename"])
		if fo.Name == "" {
			fo.Name = jsonString(envelope["name"])
		}
		return &fo, nil
	}
	if id := jsonString(envelope["file_id"]); id != "" {
		fo.ID = id
		fo.Status = jsonString(envelope["status"])
		return &fo, nil
	}
	for _, key := range []string{"data", "output", "result"} {
		if nested, ok := envelope[key]; ok && len(nested) > 0 {
			if inner, err := parseFileObject(nested); err == nil && inner.ID != "" {
				return inner, nil
			}
		}
	}
	// 顶层解析到空 id 时仍返回结构，便于上层拼错误
	_ = json.Unmarshal(raw, &fo)
	return &fo, nil
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// Content 拉取 file-extract 解析正文（Kimi 支持；通义可能不支持）。
func (c *FilesClient) Content(ctx context.Context, fileID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/files/"+fileID+"/content", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("files content HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	return string(raw), nil
}

// Retrieve 查询文件状态。
func (c *FilesClient) Retrieve(ctx context.Context, fileID string) (*FileObject, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/files/"+fileID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("files retrieve HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	var fo FileObject
	if err := json.Unmarshal(raw, &fo); err != nil {
		return nil, err
	}
	return &fo, nil
}

// WaitProcessed 轮询至 processed / error / 超时。
func (c *FilesClient) WaitProcessed(ctx context.Context, fileID string, maxWait time.Duration) (*FileObject, error) {
	if maxWait <= 0 {
		maxWait = 60 * time.Second
	}
	deadline := time.Now().Add(maxWait)
	for {
		fo, err := c.Retrieve(ctx, fileID)
		if err != nil {
			return nil, err
		}
		st := strings.ToLower(fo.Status)
		if st == "" || st == "processed" || st == "ok" {
			return fo, nil
		}
		if st == "error" || st == "failed" {
			return fo, fmt.Errorf("remote file status=%s", fo.Status)
		}
		if time.Now().After(deadline) {
			return fo, fmt.Errorf("wait file processed timeout, status=%s", fo.Status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// ChatWithFileID 通义 Qwen-Long 风格：system 里放 fileid://，请模型输出全文。
func (c *FilesClient) ChatWithFileID(ctx context.Context, model, fileID, userPrompt string) (string, error) {
	if model == "" {
		model = "qwen-long"
	}
	if userPrompt == "" {
		userPrompt = "请原样输出该文件的全部文字内容，不要总结，不要遗漏。若无法读取则只回复空字符串。"
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "fileid://" + fileID},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
	}
	rawBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat with file HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	return out.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
