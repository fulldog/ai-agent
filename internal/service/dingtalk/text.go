package dingtalk

import (
	"regexp"
	"strings"
)

var atToken = regexp.MustCompile(`@\S+`)

// onlineSearchPhrases 用户明确要求联网时的用语（长词优先，避免短词误伤）。
var onlineSearchPhrases = []string{
	"search the web",
	"search online",
	"web search",
	"google一下",
	"百度一下",
	"强制联网",
	"请联网查询",
	"请联网搜索",
	"请联网搜",
	"联网查询",
	"联网搜索",
	"联网检索",
	"在线查询",
	"在线搜索",
	"网络搜索",
	"搜索网络",
	"上网查询",
	"上网搜索",
	"上网搜",
	"上网查",
	"网上搜索",
	"网上查询",
	"网上搜",
	"网上查",
	"请联网",
	"联网搜",
	"联网查",
}

func cleanQuery(content string) string {
	s := strings.TrimSpace(content)
	s = atToken.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func isGroup(conversationType string) bool {
	return conversationType == "2"
}

func wantsOnlineSearch(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	for _, p := range onlineSearchPhrases {
		if strings.Contains(q, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func stripOnlineRequest(query string) string {
	if query == "" {
		return ""
	}
	lower := strings.ToLower(query)
	masked := []byte(query)
	for _, p := range onlineSearchPhrases {
		n := strings.ToLower(p)
		from := 0
		for {
			j := strings.Index(lower[from:], n)
			if j < 0 {
				break
			}
			start := from + j
			end := start + len(n)
			for i := start; i < end && i < len(masked); i++ {
				masked[i] = ' '
			}
			from = end
		}
	}
	return strings.Join(strings.Fields(string(masked)), " ")
}
