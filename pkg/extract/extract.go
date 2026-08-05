package extract

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fumiama/go-docx"
	"github.com/ledongthuc/pdf"
)

const maxExtractChars = 2_000_000

// Extractor extracts plain text from uploads (pdf/docx/txt/images with OCR).
type Extractor struct {
	OCR OCRConfig
}

func New(ocr OCRConfig) *Extractor {
	return &Extractor{OCR: ocr.withDefaults()}
}

// FromBytes extracts plain text from uploaded file bytes by filename extension.
func (e *Extractor) FromBytes(filename string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty file")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".xml", ".html", ".htm":
		return trimText(string(data)), nil
	case ".pdf":
		return e.extractPDF(data)
	case ".docx":
		return extractDOCX(data)
	case ".doc":
		return "", fmt.Errorf("不支持旧版 .doc 格式，请另存为 .docx 后上传")
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tif", ".tiff", ".gif":
		return OCRImageBytes(e.OCR, data, ext)
	default:
		return "", fmt.Errorf("不支持的文件类型: %s（支持 txt/md/pdf/docx/png/jpg/jpeg/webp/bmp/tif/gif）", ext)
	}
}

func (e *Extractor) extractPDF(data []byte) (string, error) {
	text, err := e.extractPDFText(data)
	if err == nil && len([]rune(text)) >= e.OCR.MinPDFTextLen {
		return text, nil
	}
	// Scanned / image-only PDF → OCR pipeline
	ocrText, ocrErr := OCRPDFScanned(e.OCR, data)
	if ocrErr == nil && ocrText != "" {
		return ocrText, nil
	}
	if text != "" {
		// Keep sparse text if OCR failed
		return text, nil
	}
	if ocrErr != nil {
		if err != nil {
			return "", fmt.Errorf("%v; OCR 回退失败: %w", err, ocrErr)
		}
		return "", ocrErr
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("pdf 中未提取到文本（可能是扫描件，请启用 OCR 并安装 tesseract + pdftoppm）")
}

func trimText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxExtractChars {
		return s[:maxExtractChars]
	}
	return s
}

// extractPDFText 优先用 Poppler pdftotext（无 CGO，与 pdftoppm 同属 poppler）；
// 不可用时回退到纯 Go 的 ledongthuc/pdf。
// 说明：go-fitz(MuPDF) 在 Windows MinGW 下链接会失败（__intrinsic_setjmpex），故不采用。
func (e *Extractor) extractPDFText(data []byte) (string, error) {
	if text, err := extractPDFTextPoppler(e.OCR, data); err == nil && text != "" {
		return text, nil
	}
	return extractPDFTextPureGo(data)
}

func extractPDFTextPureGo(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("parse pdf: %w", err)
	}
	var b strings.Builder
	numPages := r.NumPage()
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(text)
		b.WriteByte('\n')
		if b.Len() >= maxExtractChars {
			break
		}
	}
	out := trimText(b.String())
	if out == "" {
		return "", fmt.Errorf("pdf 中未提取到文本（可能是扫描件）")
	}
	return out, nil
}

func extractDOCX(data []byte) (string, error) {
	doc, err := docx.Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("parse docx: %w", err)
	}
	var b strings.Builder
	for _, item := range doc.Document.Body.Items {
		switch v := item.(type) {
		case *docx.Paragraph:
			b.WriteString(paragraphText(v))
			b.WriteByte('\n')
		case *docx.Table:
			for _, row := range v.TableRows {
				for _, cell := range row.TableCells {
					for _, p := range cell.Paragraphs {
						b.WriteString(paragraphText(p))
						b.WriteByte('\t')
					}
				}
				b.WriteByte('\n')
			}
		}
		if b.Len() >= maxExtractChars {
			break
		}
	}
	out := trimText(b.String())
	if out == "" {
		return "", fmt.Errorf("docx 中未提取到文本")
	}
	return out, nil
}

func paragraphText(p *docx.Paragraph) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	for _, child := range p.Children {
		if run, ok := child.(*docx.Run); ok {
			b.WriteString(runText(run))
		}
	}
	return b.String()
}

func runText(r *docx.Run) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, child := range r.Children {
		if t, ok := child.(*docx.Text); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// GuessName returns a document name from uploaded filename.
func GuessName(filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "" || base == "." {
		return "document"
	}
	return base
}

// IsSupportedExtension checks if file extension is supported.
func IsSupportedExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".xml", ".html", ".htm",
		".pdf", ".docx",
		".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tif", ".tiff", ".gif":
		return true
	default:
		return false
	}
}

// NormalizeQueryTerms splits query into lowercase search terms (supports CJK bigrams).
func NormalizeQueryTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	var terms []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			terms = append(terms, buf.String())
			buf.Reset()
		}
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r >= 0x4e00 && r <= 0x9fff {
			buf.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if len(terms) == 0 {
		runes := []rune(query)
		if len(runes) >= 2 {
			seen := map[string]struct{}{}
			for i := 0; i < len(runes)-1; i++ {
				term := string(runes[i : i+2])
				if _, ok := seen[term]; !ok {
					seen[term] = struct{}{}
					terms = append(terms, term)
				}
			}
		}
		if len(terms) == 0 && len(runes) > 0 {
			terms = append(terms, string(runes))
		}
	}
	return terms
}
