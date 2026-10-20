package llmprice

import "testing"

func TestCost(t *testing.T) {
	t.Parallel()
	const yuan = int64(1_000_000_000)
	tests := []struct {
		name       string
		model      string
		prompt     int64
		completion int64
		want       int64
	}{
		{
			name:   "deepseek flash input and output priced apart",
			model:  "deepseek-v4-flash",
			prompt: 1_000_000, completion: 1_000_000,
			want: 1*yuan + 4*yuan,
		},
		{
			name:   "one qwen-plus input token is 800 nano",
			model:  "qwen-plus",
			prompt: 1,
			want:   800,
		},
		{
			name:   "qwen-plus dated snapshot uses the same rate",
			model:  "qwen-plus-2025-12-01",
			prompt: 1_000_000, completion: 1_000_000,
			want: 800_000_000 + 2*yuan,
		},
		{
			name:   "unknown model is free",
			model:  "doubao-pro",
			prompt: 1_000_000, completion: 1_000_000,
			want: 0,
		},
		{
			name:   "qwen-flash-character is not qwen-flash",
			model:  "qwen-flash-character",
			prompt: 1_000_000,
			want:   0,
		},
		{
			name:       "output only",
			model:      "kimi-k3",
			completion: 1_000_000,
			want:       100 * yuan,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Cost(tt.model, tt.prompt, tt.completion)
			if got != tt.want {
				t.Fatalf("Cost(%q, %d, %d) = %d, want %d", tt.model, tt.prompt, tt.completion, got, tt.want)
			}
		})
	}
}
