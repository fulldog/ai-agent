package handler

import (
	"testing"
	"time"
)

func TestPeriodStarts(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		now       time.Time
		wantDay   string
		wantWeek  string
		wantMonth string
	}{
		{
			name:      "friday mid month",
			now:       time.Date(2026, 9, 18, 16, 54, 0, 0, loc),
			wantDay:   "2026-09-18T00:00:00+08:00",
			wantWeek:  "2026-09-14T00:00:00+08:00",
			wantMonth: "2026-09-01T00:00:00+08:00",
		},
		{
			name:      "sunday rolls back to monday",
			now:       time.Date(2026, 9, 20, 9, 0, 0, 0, loc),
			wantDay:   "2026-09-20T00:00:00+08:00",
			wantWeek:  "2026-09-14T00:00:00+08:00",
			wantMonth: "2026-09-01T00:00:00+08:00",
		},
		{
			name:      "monday is week start",
			now:       time.Date(2026, 9, 14, 0, 0, 0, 0, loc),
			wantDay:   "2026-09-14T00:00:00+08:00",
			wantWeek:  "2026-09-14T00:00:00+08:00",
			wantMonth: "2026-09-01T00:00:00+08:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			day, week, month := periodStarts(tt.now, loc)
			if day.Format(time.RFC3339) != tt.wantDay {
				t.Fatalf("day=%s want %s", day.Format(time.RFC3339), tt.wantDay)
			}
			if week.Format(time.RFC3339) != tt.wantWeek {
				t.Fatalf("week=%s want %s", week.Format(time.RFC3339), tt.wantWeek)
			}
			if month.Format(time.RFC3339) != tt.wantMonth {
				t.Fatalf("month=%s want %s", month.Format(time.RFC3339), tt.wantMonth)
			}
		})
	}
}

func TestAddBucket(t *testing.T) {
	t.Parallel()
	got := addBucket(bucketOf(1, 10, 2), bucketOf(3, 4, 5))
	if got.Calls != 4 || got.PromptTokens != 14 || got.CompletionTokens != 7 || got.TotalTokens != 21 {
		t.Fatalf("got %+v", got)
	}
}
