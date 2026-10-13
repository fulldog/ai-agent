package tools

import (
	"fmt"
	"strconv"
)

// Calc 执行 a op b，返回可读数字字符串。供 MCP 示例共用。
func Calc(a, b float64, op string) (string, error) {
	var v float64
	switch op {
	case "+":
		v = a + b
	case "-":
		v = a - b
	case "*":
		v = a * b
	case "/":
		if b == 0 {
			return "", fmt.Errorf("除数不能为 0")
		}
		v = a / b
	default:
		return "", fmt.Errorf("不支持的运算符: %s", op)
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}
