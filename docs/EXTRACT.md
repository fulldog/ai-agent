# 文档解析与 OCR

上传接口 `POST /api/v1/corpora/{id}/documents`（`multipart/form-data`，字段 `file`）支持：

| 类型 | 扩展名 | 处理方式 |
|------|--------|----------|
| 文本 | `.txt` `.md` `.csv` `.json` `.html` 等 | 直接读取 |
| Word | `.docx` | 解析正文（不支持旧版 `.doc`） |
| PDF | `.pdf` | 优先提取文字层；文字过少则 OCR |
| 图片 | `.png` `.jpg` `.jpeg` `.webp` `.bmp` `.tif` `.gif` | Tesseract OCR |

## 本机依赖（OCR）

服务通过 `exec` 调用本机二进制（无 CGO），**Linux / Windows / macOS** 均可部署，只需把可执行文件装进 PATH，或在配置中写绝对路径。

### 1. Tesseract OCR

用途：图片文字识别；扫描版 PDF 页面 OCR。需语言包 **chi_sim**（简体中文）时安装对应 tessdata。

**Linux（Debian / Ubuntu）：**

```bash
sudo apt update
sudo apt install -y tesseract-ocr tesseract-ocr-chi-sim tesseract-ocr-eng
# 验证
tesseract --version
ls /usr/share/tesseract-ocr/*/tessdata/chi_sim.traineddata
```

**Linux（RHEL / Rocky / Alma）：**

```bash
sudo dnf install -y tesseract tesseract-langpack-chi_sim tesseract-langpack-eng
# 或旧版: sudo yum install ...
```

**Windows：** [UB Mannheim 安装包](https://github.com/UB-Mannheim/tesseract/wiki)，安装时勾选 `chi_sim`，将安装目录加入 PATH，或配置绝对路径。

**macOS：** `brew install tesseract tesseract-lang`

### 2. Poppler（`pdftoppm`）

用途：仅扫描版 PDF —— 将页面转为 PNG 再交给 Tesseract。纯文字 PDF / DOCX **不需要**。

**Linux：**

```bash
sudo apt install -y poppler-utils    # Debian/Ubuntu → 提供 pdftoppm
# RHEL 系: sudo dnf install poppler-utils
pdftoppm -v
```

**Windows：** [poppler-windows Releases](https://github.com/oschwartz10612/poppler-windows/releases)，把 `Library\bin` 加入 PATH。

**macOS：** `brew install poppler`

### 配置示例

```yaml
ocr:
  enabled: true
  # Linux 常见：tesseract / /usr/bin/tesseract
  # Windows 示例：C:\Program Files\Tesseract-OCR\tesseract.exe
  tesseract_path: tesseract
  languages: chi_sim+eng
  pdftoppm_path: pdftoppm
  min_pdf_text_len: 40
  timeout_seconds: 180
```

验证（各平台命令相同）：

```bash
tesseract --version
pdftoppm -v
```

## 配置项

见 `configs/config.example.yaml` → `ocr`：

| 字段 | 说明 |
|------|------|
| `enabled` | 是否启用 OCR（图片 / 扫描 PDF） |
| `tesseract_path` | tesseract 可执行文件（命令名或绝对路径） |
| `languages` | 识别语言，默认 `chi_sim+eng` |
| `pdftoppm_path` | poppler 的 pdftoppm |
| `min_pdf_text_len` | PDF 文字少于此长度则尝试 OCR |
| `timeout_seconds` | 单次 OCR 超时 |

纯文字 PDF / DOCX **不依赖** OCR 工具。
