package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
)

func TestDictionaryUpdateRequiresDatabase(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	client := dbconn.NewDictionaryStore(filepath.Join(t.TempDir(), "srm_dictionary.json"))
	h := &DictionaryHandler{DBConn: client}
	r := gin.New()
	r.PUT("/dbconn/dictionary", h.Update)
	req := httptest.NewRequest(http.MethodPut, "/dbconn/dictionary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "业务库未连接") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDictionaryUpdateDisabled(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	h := &DictionaryHandler{}
	r := gin.New()
	r.PUT("/dbconn/dictionary", h.Update)
	req := httptest.NewRequest(http.MethodPut, "/dbconn/dictionary", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rec.Code)
	}
}
