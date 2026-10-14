package dbconn

import (
	"context"
	"fmt"
	"strings"
)

const tableNamePrefix = "Srm"

// Schema 检索表或字段。corpus 非空时只在这篇语料里查找；否则从业务库实时读取 information_schema。
func (c *Client) Schema(ctx context.Context, keyword, table, corpus string) (string, error) {
	if strings.TrimSpace(corpus) != "" {
		return lookupCorpus(corpus, keyword, table)
	}
	if strings.TrimSpace(table) != "" {
		if _, err := requireSrmTable(table); err != nil {
			return "", err
		}
	}
	if c == nil || c.db == nil {
		return "", fmt.Errorf("业务库未配置")
	}
	return c.liveSchema(ctx, keyword, table)
}

func requireSrmTable(table string) (string, error) {
	_, name, err := splitTableIdent(table)
	if err != nil {
		return "", err
	}
	if !hasTablePrefix(name) {
		return "", fmt.Errorf("仅允许扫描表名前缀为 %s 的表（区分大小写）", tableNamePrefix)
	}
	return name, nil
}
