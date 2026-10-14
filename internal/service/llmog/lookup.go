package llmog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
)

// PayloadHit 是文本日志里一条 llm_call 的完整报文。
type PayloadHit struct {
	Messages     json.RawMessage
	Response     string
	ToolCalls    json.RawMessage
	FinishReason string
}

type llmLogLine struct {
	LLMCallID    string          `json:"llm_call_id"`
	Messages     json.RawMessage `json:"messages"`
	Response     string          `json:"response"`
	ToolCalls    json.RawMessage `json:"tool_calls"`
	FinishReason string          `json:"finish_reason"`
}

// LoadPayloads 按 llm_call_logs.id 到 logs/llm-YYYY-MM-DD*.log 里回读完整报文。
// 文件缺失、单行过大或不是 JSON 时跳过，调用方用返回值是否含该 id 判断。
func LoadPayloads(dir string, rows []model.LLMCallLog) map[uuid.UUID]PayloadHit {
	out := map[uuid.UUID]PayloadHit{}
	if len(rows) == 0 || dir == "" {
		return out
	}
	want := make(map[string]uuid.UUID, len(rows))
	for _, row := range rows {
		if row.ID == uuid.Nil {
			continue
		}
		want[row.ID.String()] = row.ID
	}
	if len(want) == 0 {
		return out
	}
	for _, path := range llmLogPaths(dir, rows) {
		scanLLMLog(path, want, out)
		if len(out) == len(want) {
			return out
		}
	}
	return out
}

func llmLogPaths(dir string, rows []model.LLMCallLog) []string {
	seenDay := map[string]struct{}{}
	var days []string
	addDay := func(t time.Time) {
		if t.IsZero() {
			t = time.Now()
		}
		local := t.In(time.Local)
		for _, d := range []time.Time{local.AddDate(0, 0, -1), local, local.AddDate(0, 0, 1)} {
			key := d.Format("2006-01-02")
			if _, ok := seenDay[key]; ok {
				continue
			}
			seenDay[key] = struct{}{}
			days = append(days, key)
		}
	}
	for _, row := range rows {
		addDay(row.CreatedAt)
	}
	sort.Strings(days)

	seenPath := map[string]struct{}{}
	var paths []string
	for _, day := range days {
		matches, err := filepath.Glob(filepath.Join(dir, "llm-"+day+"*.log"))
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for _, path := range matches {
			if _, ok := seenPath[path]; ok {
				continue
			}
			seenPath[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}

func scanLLMLog(path string, want map[string]uuid.UUID, out map[uuid.UUID]PayloadHit) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, 32*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' || !bytes.Contains(line, []byte(`"llm_call_id"`)) {
			continue
		}
		var row llmLogLine
		if err := json.Unmarshal(line, &row); err != nil {
			continue
		}
		id, ok := want[row.LLMCallID]
		if !ok {
			continue
		}
		out[id] = PayloadHit{
			Messages:     append(json.RawMessage(nil), row.Messages...),
			Response:     row.Response,
			ToolCalls:    append(json.RawMessage(nil), row.ToolCalls...),
			FinishReason: row.FinishReason,
		}
		if len(out) == len(want) {
			return
		}
	}
}
