package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/filegen"
)

func TestExportDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := filegen.Open(t.TempDir(), "http://example.test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.SaveXLSX("名单", []string{"名称"}, [][]string{{"甲公司"}})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := &ExportHandler{Files: store}
	r.GET("/api/v1/exports/:id", h.Download)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+file.ID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "filename*=UTF-8''") {
		t.Fatalf("disposition %s", rec.Header().Get("Content-Disposition"))
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") {
		t.Fatalf("type %s", rec.Header().Get("Content-Type"))
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty body")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/exports/not-a-uuid", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status %d", rec.Code)
	}
}
