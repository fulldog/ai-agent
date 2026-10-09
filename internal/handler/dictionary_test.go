package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
)

func TestDictionaryUpdate(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	client := dbconn.NewDictionaryStore(filepath.Join(dir, "srm_dictionary.json"))
	h := &DictionaryHandler{DBConn: client}
	r := gin.New()
	r.PUT("/dbconn/dictionary", h.Update)

	body := `{"tables":[{"name":"Srm_New","comment":"新表","columns":[{"name":"Name","type":"varchar(8)","nullable":false,"comment":"名称"}]}]}`
	req := httptest.NewRequest(http.MethodPut, "/dbconn/dictionary", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tables":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	out, err := client.Schema(context.Background(), "", "Srm_New")
	if err != nil || !strings.Contains(out, "名称") {
		t.Fatalf("schema: %v\n%s", err, out)
	}

	bad := httptest.NewRequest(http.MethodPut, "/dbconn/dictionary", strings.NewReader(`{`))
	badRec := httptest.NewRecorder()
	r.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d body=%s", badRec.Code, badRec.Body.String())
	}
	if _, err := client.Schema(context.Background(), "", "Srm_New"); err != nil {
		t.Fatalf("invalid body must keep previous dictionary: %v", err)
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
