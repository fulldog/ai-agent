package database

import "gorm.io/gorm"

// EnsureSchemaComments 为表/特殊列补充 PostgreSQL COMMENT（GORM comment 标签会随 AutoMigrate 更新多数列）。
func EnsureSchemaComments(db *gorm.DB) error {
	stmts := []string{
		`COMMENT ON TABLE conversations IS '会话表'`,
		`COMMENT ON TABLE messages IS '会话消息表'`,
		`COMMENT ON TABLE corpora IS '语料库表'`,
		`COMMENT ON TABLE documents IS '语料文档表'`,
		`COMMENT ON TABLE chunks IS '文档分块与向量表'`,
		`COMMENT ON TABLE agent_runs IS 'Agent 运行记录表'`,
		`COMMENT ON TABLE agent_steps IS 'Agent 步骤明细表'`,
		`COMMENT ON TABLE request_logs IS 'HTTP 请求日志表'`,
		`COMMENT ON TABLE llm_call_logs IS '上游 LLM 调用日志表'`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
