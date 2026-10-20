// Package llmprice 用公开标价把 token 换成花费。
// 价格取 2026-10 的常用档：输入按缓存未命中，千问按非思考且 128K 以内，DeepSeek 按空闲价。
// 返回纳元（1 元 = 10^9）。未收录的模型返回 0。
package llmprice

import (
	"math"
	"strings"
	"unicode"
)

type rate struct {
	input  int64
	output int64
}

// 精确模型名。单价是纳元/token（1 元/百万 tokens = 1000）。vision 与 highspeed 跟对应文本模型同价。
var exact = map[string]rate{
	"deepseek-v4-flash": {input: 1000, output: 4000},
	"deepseek-flash":    {input: 1000, output: 4000},
	"deepseek-v4-pro":   {input: 4500, output: 13500},

	"moonshot-v1-8k":                  {input: 2000, output: 10000},
	"moonshot-v1-8k-vision-preview":   {input: 2000, output: 10000},
	"moonshot-v1-32k":                 {input: 5000, output: 20000},
	"moonshot-v1-32k-vision-preview":  {input: 5000, output: 20000},
	"moonshot-v1-128k":                {input: 10000, output: 30000},
	"moonshot-v1-128k-vision-preview": {input: 10000, output: 30000},
	"kimi-k2.6":                       {input: 6500, output: 27000},
	"kimi-k2.7-code":                  {input: 6500, output: 27000},
	"kimi-k2.7-code-highspeed":        {input: 6500, output: 27000},
	"kimi-k3":                         {input: 20000, output: 100000},
}

// 千问快照名（qwen-plus-2025-12-01、qwen-plus-latest）沿用同一档。
var qwenFamilies = []struct {
	name string
	rate rate
}{
	{"qwen-plus", rate{input: 800, output: 2000}},
	{"qwen-max", rate{input: 2400, output: 9600}},
	{"qwen-turbo", rate{input: 300, output: 600}},
	{"qwen-flash", rate{input: 150, output: 1500}},
	{"qwen-long", rate{input: 500, output: 2000}},
}

// Cost 返回这次调用的花费，单位纳元。未知模型或 token 不为正时返回 0。
func Cost(model string, promptTokens, completionTokens int64) int64 {
	in, out, ok := lookup(model)
	if !ok {
		return 0
	}
	return addSaturating(mulSaturating(promptTokens, in), mulSaturating(completionTokens, out))
}

func lookup(model string) (input, output int64, ok bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return 0, 0, false
	}
	if r, found := exact[model]; found {
		return r.input, r.output, true
	}
	for _, family := range qwenFamilies {
		if model == family.name || qwenSnapshot(model, family.name) {
			return family.rate.input, family.rate.output, true
		}
	}
	return 0, 0, false
}

// qwenSnapshot 识别 qwen-plus-2025-12-01 与 qwen-plus-latest，不把 qwen-flash-character 算进 qwen-flash。
func qwenSnapshot(model, family string) bool {
	rest, found := strings.CutPrefix(model, family+"-")
	if !found || rest == "" {
		return false
	}
	if rest == "latest" {
		return true
	}
	if len(rest) < 10 || rest[4] != '-' || rest[7] != '-' {
		return false
	}
	if !digits(rest[:4]) || !digits(rest[5:7]) || !digits(rest[8:10]) {
		return false
	}
	if len(rest) == 10 {
		return true
	}
	return rest[10] == '-'
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func mulSaturating(tokens, nanoPerToken int64) int64 {
	if tokens <= 0 || nanoPerToken <= 0 {
		return 0
	}
	if tokens > math.MaxInt64/nanoPerToken {
		return math.MaxInt64
	}
	return tokens * nanoPerToken
}

func addSaturating(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
