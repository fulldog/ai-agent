package filegen

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

const (
	maxColumns   = 40
	maxRows      = 5000
	maxCellRunes = 2000
)

var plainNumber = regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+)?$`)

func buildXLSX(title string, columns []string, rows [][]string) ([]byte, string, error) {
	if len(columns) == 0 {
		return nil, "", fmt.Errorf("columns 不能为空")
	}
	if len(columns) > maxColumns {
		return nil, "", fmt.Errorf("列数超过 %d", maxColumns)
	}
	if len(rows) == 0 {
		return nil, "", fmt.Errorf("没有可写入的数据行")
	}
	if len(rows) > maxRows {
		return nil, "", fmt.Errorf("行数超过 %d", maxRows)
	}
	name := cleanFilename(title)
	sheet := sheetName(strings.TrimSuffix(name, ".xlsx"))

	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, "", err
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F4E79"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, "", err
	}
	bodyStyle, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "D9D9D9", Style: 1},
			{Type: "right", Color: "D9D9D9", Style: 1},
			{Type: "top", Color: "D9D9D9", Style: 1},
			{Type: "bottom", Color: "D9D9D9", Style: 1},
		},
	})
	if err != nil {
		return nil, "", err
	}
	altStyle, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"F3F6F9"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "D9D9D9", Style: 1},
			{Type: "right", Color: "D9D9D9", Style: 1},
			{Type: "top", Color: "D9D9D9", Style: 1},
			{Type: "bottom", Color: "D9D9D9", Style: 1},
		},
	})
	if err != nil {
		return nil, "", err
	}

	widths := make([]int, len(columns))
	for i, col := range columns {
		col = clipCell(col)
		columns[i] = col
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, "", err
		}
		if err := f.SetCellStr(sheet, cell, col); err != nil {
			return nil, "", err
		}
		if err := f.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return nil, "", err
		}
		widths[i] = displayWidth(col)
	}
	for r, row := range rows {
		style := bodyStyle
		if r%2 == 1 {
			style = altStyle
		}
		for c := 0; c < len(columns); c++ {
			val := ""
			if c < len(row) {
				val = clipCell(row[c])
			}
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				return nil, "", err
			}
			if err := setDataCell(f, sheet, cell, val); err != nil {
				return nil, "", err
			}
			if err := f.SetCellStyle(sheet, cell, cell, style); err != nil {
				return nil, "", err
			}
			if w := displayWidth(val); w > widths[c] {
				widths[c] = w
			}
		}
	}
	for i, w := range widths {
		if w < 10 {
			w = 10
		}
		if w > 36 {
			w = 36
		}
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, "", err
		}
		if err := f.SetColWidth(sheet, col, col, float64(w+2)); err != nil {
			return nil, "", err
		}
	}
	end, err := excelize.CoordinatesToCellName(len(columns), len(rows)+1)
	if err != nil {
		return nil, "", err
	}
	_ = f.AutoFilter(sheet, "A1:"+end, nil)
	_ = f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})
	_ = f.SetRowHeight(sheet, 1, 22)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), name, nil
}

func setDataCell(f *excelize.File, sheet, cell, val string) error {
	if n, ok := asNumber(val); ok {
		return f.SetCellFloat(sheet, cell, n, -1, 64)
	}
	return f.SetCellStr(sheet, cell, escapeFormula(val))
}

func asNumber(s string) (float64, bool) {
	if !plainNumber.MatchString(s) {
		return 0, false
	}
	intPart := strings.TrimPrefix(s, "-")
	if dot := strings.IndexByte(intPart, '.'); dot >= 0 {
		intPart = intPart[:dot]
	}
	if len(intPart) > 15 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func escapeFormula(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	default:
		return s
	}
}

func clipCell(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	if utf8.RuneCountInString(s) <= maxCellRunes {
		return s
	}
	rs := []rune(s)
	return string(rs[:maxCellRunes])
}

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		if r > 0x7f {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func cleanFilename(name string) string {
	name = strings.TrimSpace(name)
	if ext := strings.ToLower(extOf(name)); ext == ".xlsx" || ext == ".xls" {
		name = name[:len(name)-len(ext)]
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(`\/:*?"<>|`, r) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	name = strings.Trim(b.String(), " ._")
	rs := []rune(name)
	if len(rs) > 80 {
		rs = rs[:80]
	}
	name = strings.Trim(string(rs), " ._")
	if name == "" {
		name = "导出结果"
	}
	return name + ".xlsx"
}

func extOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return ""
	}
	return name[i:]
}

func sheetName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`[]:*?/\\`, r) || r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	name = strings.Trim(b.String(), " '")
	rs := []rune(name)
	if len(rs) > 31 {
		rs = rs[:31]
	}
	name = strings.TrimSpace(string(rs))
	if name == "" {
		return "数据"
	}
	return name
}
