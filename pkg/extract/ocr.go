package extract

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// OCRConfig controls external OCR tools (no CGO).
type OCRConfig struct {
	Enabled        bool
	TesseractPath  string // default: tesseract
	Languages      string // default: chi_sim+eng
	PDFToPPMPath   string // default: pdftoppm (poppler); used for scanned PDF
	MinPDFTextLen  int    // if extracted PDF text shorter than this, try OCR
	TimeoutSeconds int
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
	if c.MinPDFTextLen <= 0 {
		c.MinPDFTextLen = 40
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 180
	}
	return c
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
	ctxTimeout := timeout
	cmd := exec.Command(cfg.TesseractPath, imagePath, "stdout", "-l", cfg.Languages, "--psm", "3")
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
	case <-time.After(ctxTimeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("tesseract OCR 超时（%ds）", cfg.TimeoutSeconds)
	}
	out := trimText(stdout.String())
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
	cmd := exec.Command(cfg.PDFToPPMPath, "-png", "-r", "200", pdfPath, prefix)
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
