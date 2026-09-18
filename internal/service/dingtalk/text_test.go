package dingtalk

import "testing"

func TestCleanQuery(t *testing.T) {
	got := cleanQuery("  @财务助手 报销流程怎么走  @张三 ")
	if got != "报销流程怎么走" {
		t.Fatalf("got %q", got)
	}
}

func TestIsGroup(t *testing.T) {
	if !isGroup("2") || isGroup("1") {
		t.Fatal("conversationType 2 is group, 1 is DM")
	}
}

func TestWantsOnlineSearch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "plain question", query: "报销流程怎么走", want: false},
		{name: "search corpus wording", query: "搜索一下年假天数", want: false},
		{name: "explicit 联网查询", query: "联网查询今天天气", want: true},
		{name: "explicit 请联网", query: "请联网 年假规定", want: true},
		{name: "web search english", query: "Web Search Q3 SLA", want: true},
		{name: "empty", query: "  ", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := wantsOnlineSearch(tt.query); got != tt.want {
				t.Fatalf("wantsOnlineSearch(%q)=%v want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestStripOnlineRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "prefix", query: "联网查询报销流程", want: "报销流程"},
		{name: "please online", query: "请联网查询 年假几天", want: "年假几天"},
		{name: "no marker", query: "报销流程", want: "报销流程"},
		{name: "only marker", query: "联网查询", want: ""},
		{name: "english", query: "Web Search Q3 SLA", want: "Q3 SLA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stripOnlineRequest(tt.query); got != tt.want {
				t.Fatalf("stripOnlineRequest(%q)=%q want %q", tt.query, got, tt.want)
			}
		})
	}
}
