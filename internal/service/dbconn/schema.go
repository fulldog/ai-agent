package dbconn

import (
	"context"
	"fmt"
	"strings"
)

const tableNamePrefix = "Srm"

// Schema 读取表结构。corpus 非空时按这篇数据字典返回：指定 table 返回该表，指定 keyword 筛选，都不指定则返回规则和表目录。否则从业务库实时读取 information_schema。
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
