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
