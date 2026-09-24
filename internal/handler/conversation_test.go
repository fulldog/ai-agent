package handler

import "testing"

func TestIncludeDeletedQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"YES", true},
		{" True ", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := includeDeletedQuery(tc.in); got != tc.want {
				t.Fatalf("includeDeletedQuery(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}
