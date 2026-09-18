package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/middleware"
	"gorm.io/gorm"
)

const usageTimezone = "Asia/Shanghai"

type tokenBucket struct {
	Calls            int64 `json:"calls"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type tokenUsageItem struct {
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	All      tokenBucket `json:"all"`
	Day      tokenBucket `json:"day"`
	Week     tokenBucket `json:"week"`
	Month    tokenBucket `json:"month"`
}

type tokenUsageScan struct {
	Provider              string
	Model                 string
	Calls                 int64
	PromptTokens          int64
	CompletionTokens      int64
	DayCalls              int64
	DayPromptTokens       int64
	DayCompletionTokens   int64
	WeekCalls             int64
	WeekPromptTokens      int64
	WeekCompletionTokens  int64
	MonthCalls            int64
	MonthPromptTokens     int64
	MonthCompletionTokens int64
}

func usageLocation() *time.Location {
	loc, err := time.LoadLocation(usageTimezone)
	if err != nil {
		return time.Local
	}
	return loc
}

func periodStarts(now time.Time, loc *time.Location) (day, week, month time.Time) {
	now = now.In(loc)
	day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	offset := int(now.Weekday() - time.Monday)
	if offset < 0 {
		offset += 7
	}
	week = day.AddDate(0, 0, -offset)
	month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	return day, week, month
}

func bucketOf(calls, prompt, completion int64) tokenBucket {
	return tokenBucket{
		Calls:            calls,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
	}
}

func addBucket(a, b tokenBucket) tokenBucket {
	return bucketOf(a.Calls+b.Calls, a.PromptTokens+b.PromptTokens, a.CompletionTokens+b.CompletionTokens)
}

func queryTokenUsage(db *gorm.DB, uid string, day, week, month time.Time) ([]tokenUsageScan, error) {
	q := `
SELECT
  COALESCE(provider, '') AS provider,
  COALESCE(model, '') AS model,
  COUNT(*) AS calls,
  COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
  COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
  COUNT(*) FILTER (WHERE created_at >= ?) AS day_calls,
  COALESCE(SUM(prompt_tokens) FILTER (WHERE created_at >= ?), 0) AS day_prompt_tokens,
  COALESCE(SUM(completion_tokens) FILTER (WHERE created_at >= ?), 0) AS day_completion_tokens,
  COUNT(*) FILTER (WHERE created_at >= ?) AS week_calls,
  COALESCE(SUM(prompt_tokens) FILTER (WHERE created_at >= ?), 0) AS week_prompt_tokens,
  COALESCE(SUM(completion_tokens) FILTER (WHERE created_at >= ?), 0) AS week_completion_tokens,
  COUNT(*) FILTER (WHERE created_at >= ?) AS month_calls,
  COALESCE(SUM(prompt_tokens) FILTER (WHERE created_at >= ?), 0) AS month_prompt_tokens,
  COALESCE(SUM(completion_tokens) FILTER (WHERE created_at >= ?), 0) AS month_completion_tokens
FROM llm_call_logs`
	args := []any{day, day, day, week, week, week, month, month, month}
	if uid != "" {
		q += ` WHERE request_id IN (SELECT request_id FROM request_logs WHERE uid = ?)`
		args = append(args, uid)
	}
	q += `
GROUP BY provider, model
ORDER BY (COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0)) DESC, provider, model`
	var rows []tokenUsageScan
	err := db.Raw(q, args...).Scan(&rows).Error
	return rows, err
}

func (h *LogsHandler) TokenUsage(c *gin.Context) {
	admin := middleware.IsAdminContext(c)
	filter := ""
	if admin {
		filter = strings.TrimSpace(c.Query("uid"))
	} else {
		uid, ok := requireUID(c)
		if !ok {
			return
		}
		filter = uid
	}
	loc := usageLocation()
	day, week, month := periodStarts(time.Now(), loc)
	scans, err := queryTokenUsage(h.DB, filter, day, week, month)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	items := make([]tokenUsageItem, 0, len(scans))
	var totals tokenUsageItem
	for _, s := range scans {
		item := tokenUsageItem{
			Provider: s.Provider,
			Model:    s.Model,
			All:      bucketOf(s.Calls, s.PromptTokens, s.CompletionTokens),
			Day:      bucketOf(s.DayCalls, s.DayPromptTokens, s.DayCompletionTokens),
			Week:     bucketOf(s.WeekCalls, s.WeekPromptTokens, s.WeekCompletionTokens),
			Month:    bucketOf(s.MonthCalls, s.MonthPromptTokens, s.MonthCompletionTokens),
		}
		items = append(items, item)
		totals.All = addBucket(totals.All, item.All)
		totals.Day = addBucket(totals.Day, item.Day)
		totals.Week = addBucket(totals.Week, item.Week)
		totals.Month = addBucket(totals.Month, item.Month)
	}
	c.JSON(http.StatusOK, gin.H{
		"timezone":    loc.String(),
		"day_from":    day.Format(time.RFC3339),
		"week_from":   week.Format(time.RFC3339),
		"month_from":  month.Format(time.RFC3339),
		"items":       items,
		"totals":      totals,
		"scope_admin": admin,
	})
}
