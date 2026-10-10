package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAddDocumentsDoesNotMergeFiles(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for _, name := range []string{"a.txt", "b.txt"} {
		part, err := w.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	h := &CorpusHandler{}
	r := gin.New()
	r.POST("/corpora/:id/documents", h.AddDocument)
	req := httptest.NewRequest(http.MethodPost, "/corpora/"+uuid.NewString()+"/documents", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []struct {
			Filename string `json:"filename"`
			Error    string `json:"error"`
			Document any    `json:"document"`
		} `json:"items"`
		OK     int `json:"ok"`
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK != 0 || got.Failed != 2 || len(got.Items) != 2 {
		t.Fatalf("want 2 separate failures, got %+v", got)
	}
	if got.Items[0].Filename != "a.txt" || got.Items[1].Filename != "b.txt" {
		t.Fatalf("filenames merged or reordered: %+v", got.Items)
	}
	if got.Items[0].Document != nil || got.Items[1].Document != nil {
		t.Fatalf("files were indexed as documents: %+v", got.Items)
	}
	for _, it := range got.Items {
		if it.Error == "" {
			t.Fatalf("missing per-file error: %+v", it)
		}
	}
}

func TestReuploadDocumentRequiresFile(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	h := &CorpusHandler{}
	r := gin.New()
	r.POST("/corpora/:id/documents/:doc_id/reupload", h.ReuploadDocument)
	req := httptest.NewRequest(http.MethodPost, "/corpora/"+uuid.NewString()+"/documents/"+uuid.NewString()+"/reupload", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("file required")) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
