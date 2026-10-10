package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/dingtalk"
	"go.uber.org/zap"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func TestReplyModeHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:TestReplyModeHTTP?mode=memory&cache=shared"), &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.DingTalk.ReplyMode = config.DingTalkReplyChat
	bot := dingtalk.New(cfg, nil, nil, nil, nil, zap.NewNop(), zap.NewNop(), db)

	h := &DingTalkHandler{Bot: bot}
	r := gin.New()
	r.GET("/dingtalk/reply-mode", h.GetReplyMode)
	r.PUT("/dingtalk/reply-mode", h.SetReplyMode)

	get := httptest.NewRequest(http.MethodGet, "/dingtalk/reply-mode", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, get)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reply_mode":"chat"`) {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/dingtalk/reply-mode", strings.NewReader(`{"reply_mode":"web"}`))
	put.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, put)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reply_mode":"web"`) || !strings.Contains(rec.Body.String(), `"from_store":true`) {
		t.Fatalf("put status=%d body=%s", rec.Code, rec.Body.String())
	}

	bad := httptest.NewRequest(http.MethodPut, "/dingtalk/reply-mode", strings.NewReader(`{"reply_mode":"nope"}`))
	bad.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, bad)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReplyModeHTTPUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DingTalkHandler{}
	r := gin.New()
	r.PUT("/dingtalk/reply-mode", h.SetReplyMode)
	req := httptest.NewRequest(http.MethodPut, "/dingtalk/reply-mode", strings.NewReader(`{"reply_mode":"chat"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
