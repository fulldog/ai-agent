package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

// Tool 单个可调用工具。Description 与参数说明使用中文；Name 为英文 snake_case。
type Tool interface {
	Name() string
	Spec() llm.ToolSpec
	Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error)
}

// Env 执行期注入（语料、下游服务等）。
type Env struct {
	CorpusID    *uuid.UUID
	CorpusIDs   []uuid.UUID
	TopK        int
	DefaultTopK int
	// RAGQuery 固定为本轮用户输入；非空时 knowledge_search 不采用模型改写的查询词。
	RAGQuery string
	// RAGDisabled 为 true 时不检索知识库（未绑定语料库的钉钉单聊）。
	RAGDisabled bool
	RAG         *rag.Service
	// query 是本次运行里最近一次 dbconn query 的完整结果，供 generate_file 直接导出。
	query *QueryTable
	// SaveSQL 在 dbconn 执行语句后调用，用于写入会话历史。为空则不记。
	SaveSQL func(sql string)
	// Dictionary 是当前会话配置的数据字典语料原文。空则 schema 实时读库。
	Dictionary string
}

// RecordExecutedSQL 记下刚执行的语句。sql 为空时忽略。
func (e *Env) RecordExecutedSQL(sqlText string) {
	if e == nil || e.SaveSQL == nil {
		return
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return
	}
	e.SaveSQL(sqlText)
}

// QueryTable 一次查询读到的表。Rows 与 Columns 对齐。
type QueryTable struct {
	Columns   []string
	Rows      [][]string
	Truncated bool
}

// SetQueryTable 记下最近一次查询。后续 generate_file 用 from_query 导出时读这份副本。
func (e *Env) SetQueryTable(t QueryTable) {
	if e == nil {
		return
	}
	cp := QueryTable{
		Columns:   append([]string(nil), t.Columns...),
		Rows:      make([][]string, len(t.Rows)),
		Truncated: t.Truncated,
	}
	for i, row := range t.Rows {
		cp.Rows[i] = append([]string(nil), row...)
	}
	e.query = &cp
}

// QueryTable 返回最近一次查询。没有查询时 ok 为 false。
func (e *Env) QueryTable() (QueryTable, bool) {
	if e == nil || e.query == nil {
		return QueryTable{}, false
	}
	return *e.query, true
}
