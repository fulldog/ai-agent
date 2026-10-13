package dbconn

import "testing"

func TestCellText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   any
		want string
	}{
		{in: nil, want: ""},
		{in: "北京字节跳动科技有限公司", want: "北京字节跳动科技有限公司"},
		{in: true, want: "是"},
		{in: false, want: "否"},
		{in: int64(10000001), want: "10000001"},
		{in: float64(1.5), want: "1.5"},
	}
	for _, tt := range tests {
		if got := cellText(tt.in); got != tt.want {
			t.Fatalf("cellText(%#v)=%q", tt.in, got)
		}
	}
}
