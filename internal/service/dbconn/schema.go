package dbconn

import (
	"context"
	"fmt"
	"strings"
)

const tableNamePrefix = "Srm"

// Schema 从本地数据字典检索表或字段，不访问数据库。
func (c *Client) Schema(ctx context.Context, keyword, table string) (string, error) {
	_ = ctx
	d := c.dictionary()
	if d == nil {
		return "", fmt.Errorf("业务库未配置")
	}
	return d.lookup(strings.TrimSpace(keyword), strings.TrimSpace(table))
}

func (c *Client) dictionary() *Dictionary {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dict
}
