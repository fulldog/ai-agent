package dingtalk

import (
	"regexp"
	"strings"
)

var atToken = regexp.MustCompile(`@\S+`)

func cleanQuery(content string) string {
	s := strings.TrimSpace(content)
	s = atToken.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func isGroup(conversationType string) bool {
	return conversationType == "2"
}
