package dbconn

import (
	"fmt"
	"strings"
)

// lookupCorpus 返回数据字典语料全文。keyword、table 不参与截取。
func lookupCorpus(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("语料为空")
	}
	return text, nil
}
