package extract

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// OCRConfig controls external OCR / PDF tools (no CGO).
type OCRConfig struct {
	Enabled        bool
	TesseractPath  string // default: tesseract
	Languages      string // default: chi_sim+eng
	PDFToPPMPath   string // default: pdftoppm (poppler); used for scanned PDF
	PDFToTextPath  string // default: pdftotext (poppler); used for PDF text layer
	MinPDFTextLen  int    // if extracted PDF text shorter than this, try OCR
	TimeoutSeconds int
	// DPI: pdftoppm -r。默认 200（与改参前一致）；印章多的扫描件盲目提 DPI/灰度可能更差。
	DPI int
	// PSM: tesseract --psm。默认 3=全自动；6=整页均匀块（有印章/页眉时易更差）。
	PSM int
	// OEM: tesseract --oem。3=默认（LSTM）。配置为 0 时按 3 处理。
	OEM int
	// PDFToPPMGray: pdftoppm -gray；默认 false（彩色扫描合同通常更稳）。
	PDFToPPMGray bool
	// CollapseCJKSpaces: 去掉汉字之间的多余空格。默认 true。
	CollapseCJKSpaces *bool
}

func (c OCRConfig) withDefaults() OCRConfig {
	if c.TesseractPath == "" {
		c.TesseractPath = "tesseract"
	}
	if c.Languages == "" {
		c.Languages = "chi_sim+eng"
	}
	if c.PDFToPPMPath == "" {
		c.PDFToPPMPath = "pdftoppm"
	}
	if c.PDFToTextPath == "" {
		c.PDFToTextPath = "pdftotext"
	}
	if c.MinPDFTextLen <= 0 {
		c.MinPDFTextLen = 40
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 180
	}
	if c.DPI <= 0 {
		c.DPI = 200
	}
	if c.PSM <= 0 {
		c.PSM = 3
	}
	if c.OEM <= 0 {
		c.OEM = 3
	}
	if c.CollapseCJKSpaces == nil {
		v := true
		c.CollapseCJKSpaces = &v
	}
	return c
}

func (c OCRConfig) collapseSpaces() bool {
	return c.CollapseCJKSpaces == nil || *c.CollapseCJKSpaces
}

// extractPDFTextPoppler 调用 poppler 的 pdftotext 抽取文字层（无 CGO）。
func extractPDFTextPoppler(cfg OCRConfig, data []byte) (string, error) {
	cfg = cfg.withDefaults()
	dir, err := os.MkdirTemp("", "ai-agent-pdf-text-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	pdfPath := filepath.Join(dir, "input.pdf")
	outPath := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
		return "", err
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// -layout 尽量保留阅读顺序；-enc UTF-8 输出中文
	cmd := exec.CommandContext(ctx, cfg.PDFToTextPath, "-layout", "-enc", "UTF-8", pdfPath, outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("pdftotext 失败: %s", msg)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		return "", err
	}
	out := trimText(string(b))
	if out == "" {
		return "", fmt.Errorf("pdftotext 未提取到文本")
	}
	return out, nil
}

// OCRImageBytes runs tesseract on an image file written to a temp path.
func OCRImageBytes(cfg OCRConfig, data []byte, ext string) (string, error) {
	cfg = cfg.withDefaults()
	if !cfg.Enabled {
		return "", fmt.Errorf("OCR 未启用（请在配置中设置 ocr.enabled: true，并安装 tesseract）")
	}
	ext = strings.ToLower(ext)
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	dir, err := os.MkdirTemp("", "ai-agent-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	imgPath := filepath.Join(dir, "input"+ext)
	if err := os.WriteFile(imgPath, data, 0o644); err != nil {
		return "", err
	}
	return runTesseract(cfg, imgPath)
}

func runTesseract(cfg OCRConfig, imagePath string) (string, error) {
	cfg = cfg.withDefaults()
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	args := []string{
		imagePath, "stdout",
		"-l", cfg.Languages,
		"--psm", strconv.Itoa(cfg.PSM),
		"--oem", strconv.Itoa(cfg.OEM),
	}
	cmd := exec.Command(cfg.TesseractPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", fmt.Errorf("tesseract OCR 失败: %s（请确认已安装 tesseract，且 PATH 可访问，语言包含 %s）", msg, cfg.Languages)
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("tesseract OCR 超时（%ds）", cfg.TimeoutSeconds)
	}
	out := trimText(stdout.String())
	if cfg.collapseSpaces() {
		out = collapseCJKSpaces(out)
	}
	if out == "" {
		return "", fmt.Errorf("OCR 未识别到文字")
	}
	return out, nil
}

// OCRPDFScanned rasterizes PDF pages via pdftoppm then OCRs each page.
func OCRPDFScanned(cfg OCRConfig, data []byte) (string, error) {
	cfg = cfg.withDefaults()
	if !cfg.Enabled {
		return "", fmt.Errorf("扫描版 PDF 需要 OCR，但 ocr.enabled=false")
	}
	dir, err := os.MkdirTemp("", "ai-agent-pdf-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	pdfPath := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
		return "", err
	}
	prefix := filepath.Join(dir, "page")
	args := []string{"-png", "-r", strconv.Itoa(cfg.DPI)}
	if cfg.PDFToPPMGray {
		args = append(args, "-gray")
	}
	args = append(args, pdfPath, prefix)
	cmd := exec.Command(cfg.PDFToPPMPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("pdftoppm 转图片失败: %s（扫描版 PDF OCR 需要安装 poppler 的 pdftoppm）", msg)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var pages []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "page") && strings.HasSuffix(strings.ToLower(name), ".png") {
			pages = append(pages, filepath.Join(dir, name))
		}
	}
	sort.Strings(pages)
	if len(pages) == 0 {
		return "", fmt.Errorf("pdftoppm 未生成页面图片")
	}

	var b strings.Builder
	for i, p := range pages {
		text, err := runTesseract(cfg, p)
		if err != nil {
			return "", fmt.Errorf("第 %d 页 OCR 失败: %w", i+1, err)
		}
		b.WriteString(text)
		b.WriteByte('\n')
		if b.Len() >= maxExtractChars {
			break
		}
	}
	return trimText(b.String()), nil
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK 标点
		(r >= 0xFF00 && r <= 0xFFEF) // 全角
}

// collapseCJKSpaces 去掉汉字/全角字符之间的多余空格，保留英文词间空格。
func collapseCJKSpaces(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\u3000", " ")
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == ' ' {
			prev, next := rune(0), rune(0)
			if i > 0 {
				prev = runes[i-1]
			}
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if shouldDropSpace(prev, next) {
				continue
			}
			// 连续空格压成一个（英文词间）
			if prev == ' ' {
				continue
			}
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func shouldDropSpace(prev, next rune) bool {
	if prev == 0 || next == 0 || next == '\n' || prev == '\n' {
		return false
	}
	// 汉字—汉字 / 汉字—数字 / 数字—汉字 / 汉字—标点 等去空格
	if isCJK(prev) && isCJK(next) {
		return true
	}
	if isCJK(prev) && (unicode.IsDigit(next) || unicode.IsPunct(next)) {
		return true
	}
	if (unicode.IsDigit(prev) || unicode.IsPunct(prev)) && isCJK(next) {
		return true
	}
	if unicode.IsDigit(prev) && unicode.IsDigit(next) {
		return true
	}
	if isCJK(prev) && unicode.IsLetter(next) {
		return true
	}
	if unicode.IsLetter(prev) && isCJK(next) {
		return true
	}
	return false
}
