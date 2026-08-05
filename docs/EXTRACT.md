# 文档解析与 OCR

上传接口 `POST /api/v1/corpora/{id}/documents`、以及 `POST /api/v1/chat/analyze` 等支持：

| 类型 | 扩展名 | 处理方式 |
|------|--------|----------|
| 文本 | `.txt` `.md` `.csv` `.json` `.html` 等 | 直接读取 |
| Word | `.docx` | 解析正文（不支持旧版 `.doc`） |
| PDF | `.pdf` | 优先 Poppler `pdftotext` 抽文字层（无 CGO）；不可用时回退纯 Go `ledongthuc/pdf`；文字过少则 OCR |
| 图片 | `.png` `.jpg` `.jpeg` `.webp` `.bmp` `.tif` `.gif` | Tesseract OCR |

> 不使用 `go-fitz`：其捆绑的 MuPDF 静态库与 Windows MinGW 链接会报 `__intrinsic_setjmpex`。  
> 不使用 `pdfcpu`：star 虽高，但不做明文抽取。  
> **强烈推荐安装 Poppler**（`pdftotext` / `pdftoppm`）与 **Tesseract**（扫描件 / 图片 OCR，含 `chi_sim`）。

服务通过 `exec` 调用本机二进制（无 CGO），**Linux / Windows / macOS** 均可部署，只需把可执行文件装进 PATH，或在配置中写绝对路径。

---

## 1. 安装 Poppler

本项目用到的 Poppler 工具：

| 可执行文件 | 用途 | 何时需要 |
|------------|------|----------|
| `pdftotext` | 抽取 PDF 文字层 | 合同/文档分析、语料上传（推荐始终安装） |
| `pdftoppm` | PDF 页转 PNG | 仅扫描版 / 图片型 PDF 走 OCR 时需要 |

二者通常在同一个安装包里。

### 1.1 Windows

1. 打开发布页：[poppler-windows Releases](https://github.com/oschwartz10612/poppler-windows/releases)  
2. 下载最新 zip（如 `Release-xx.xx.x-x.zip`）  
3. 解压到固定目录，例如：`C:\tools\poppler\` 或 `C:\Poppler\poppler-26.02.0\`  
4. 将 **`Library\bin`** 加入系统 / 用户环境变量 **PATH**，例如：  
   `C:\Poppler\poppler-26.02.0\Library\bin`  
5. **重新打开** PowerShell / 终端后验证：

```powershell
pdftotext -v
pdftoppm -v
where.exe pdftotext
where.exe pdftoppm
```

能看到版本号即安装成功。

若不改 PATH，可在 `configs/config.yaml` 写绝对路径（**推荐**，避免服务进程未继承新 PATH）：

```yaml
ocr:
  pdftotext_path: "C:\\Poppler\\poppler-26.02.0\\Library\\bin\\pdftotext.exe"
  pdftoppm_path: "C:\\Poppler\\poppler-26.02.0\\Library\\bin\\pdftoppm.exe"
```

### 1.2 Linux（Debian / Ubuntu）

```bash
sudo apt update
sudo apt install -y poppler-utils
pdftotext -v
pdftoppm -v
which pdftotext pdftoppm
```

### 1.3 Linux（RHEL / Rocky / Alma）

```bash
sudo dnf install -y poppler-utils
# 或旧版: sudo yum install -y poppler-utils
pdftotext -v
pdftoppm -v
```

### 1.4 macOS

```bash
brew install poppler
pdftotext -v
pdftoppm -v
```

---

## 2. 安装 Tesseract OCR

用途：图片文字识别；**扫描版 / 纯图片 PDF**（先经 `pdftoppm` 转图）页面 OCR。中文文档必须安装语言包 **chi_sim**（简体中文）。

### 2.1 Windows（推荐 UB Mannheim）

#### 方式 A：winget（命令行）

```powershell
winget install --id UB-Mannheim.TesseractOCR -e --accept-package-agreements --accept-source-agreements
```

默认安装目录：`C:\Program Files\Tesseract-OCR\`。

winget 静默安装往往**只有 eng**，需手动补中文语言包（见下方「安装 chi_sim」）。

#### 方式 B：图形安装包

1. 打开：[UB Mannheim Tesseract wiki](https://github.com/UB-Mannheim/tesseract/wiki)  
2. 下载 Windows 安装包并运行  
3. 安装向导中勾选语言包：**Chinese - Simplified**（`chi_sim`），建议同时保留 English  
4. 勾选将安装目录加入 PATH（或稍后手动加入）  
5. 默认路径通常为：`C:\Program Files\Tesseract-OCR\`

#### 将 Tesseract 加入 PATH

把安装目录加入用户 PATH，例如：

`C:\Program Files\Tesseract-OCR`

然后**新开终端**验证：

```powershell
tesseract --version
tesseract --list-langs
where.exe tesseract
```

`--list-langs` 中应能看到 `chi_sim` 与 `eng`。

#### 安装 / 补齐 chi_sim（简体中文）

若 `tesseract --list-langs` **没有** `chi_sim`，下载语言数据到 `tessdata` 目录：

```powershell
# 目标目录（按实际安装路径调整）
$dir = "C:\Program Files\Tesseract-OCR\tessdata"
$url = "https://github.com/tesseract-ocr/tessdata/raw/main/chi_sim.traineddata"
Invoke-WebRequest -Uri $url -OutFile "$dir\chi_sim.traineddata"
tesseract --list-langs
```

也可浏览器下载 [chi_sim.traineddata](https://github.com/tesseract-ocr/tessdata/raw/main/chi_sim.traineddata)，放到：

`C:\Program Files\Tesseract-OCR\tessdata\chi_sim.traineddata`

#### 配置文件写绝对路径（推荐）

```yaml
ocr:
  enabled: true
  tesseract_path: "C:\\Program Files\\Tesseract-OCR\\tesseract.exe"
  languages: chi_sim+eng
```

### 2.2 Linux（Debian / Ubuntu）

```bash
sudo apt update
sudo apt install -y tesseract-ocr tesseract-ocr-chi-sim tesseract-ocr-eng
tesseract --version
tesseract --list-langs
ls /usr/share/tesseract-ocr/*/tessdata/chi_sim.traineddata
```

### 2.3 Linux（RHEL / Rocky / Alma）

```bash
sudo dnf install -y tesseract tesseract-langpack-chi_sim tesseract-langpack-eng
# 或旧版: sudo yum install ...
tesseract --version
tesseract --list-langs
```

### 2.4 macOS

```bash
brew install tesseract tesseract-lang
tesseract --version
tesseract --list-langs
```

---

## 3. 配置示例

```yaml
ocr:
  enabled: true
  tesseract_path: "C:\\Program Files\\Tesseract-OCR\\tesseract.exe"  # Linux 可用 tesseract
  languages: chi_sim+eng
  pdftotext_path: "C:\\Poppler\\poppler-26.02.0\\Library\\bin\\pdftotext.exe"
  pdftoppm_path: "C:\\Poppler\\poppler-26.02.0\\Library\\bin\\pdftoppm.exe"
  min_pdf_text_len: 40
  timeout_seconds: 180
  dpi: 200                  # 转图分辨率
  psm: 3                    # 页面分割模式
  oem: 3                    # OCR 引擎
  pdftoppm_gray: false      # 是否灰度转图
  collapse_cjk_spaces: true # 清洗汉字间空格
```

验证（各平台；Windows 若 PATH 未刷新可用绝对路径）：

```powershell
tesseract --version
tesseract --list-langs
pdftotext -v
pdftoppm -v
```

---

## 4. 配置项

见 `configs/config.example.yaml` → `ocr`：

| 字段 | 默认 | 说明 |
|------|------|------|
| `enabled` | `true` | 是否启用 OCR（图片 / 扫描 PDF） |
| `tesseract_path` | `tesseract` | tesseract 可执行文件（命令名或绝对路径） |
| `languages` | `chi_sim+eng` | 识别语言；中文必须含 `chi_sim` |
| `pdftotext_path` | `pdftotext` | poppler：PDF 文字层抽取 |
| `pdftoppm_path` | `pdftoppm` | poppler：扫描 PDF 转 PNG |
| `pdftoppm_gray` | `false` | 是否加 `-gray`。带红章的扫描合同默认 **false（彩色）** 更稳 |
| `min_pdf_text_len` | `40` | `pdftotext` 结果短于此则走 OCR |
| `timeout_seconds` | `180` | 单次 tesseract / 抽字超时（秒） |
| `dpi` | `200` | `pdftoppm -r`。可试 300；印章/污损页提高 DPI 不一定更好 |
| `psm` | `3` | `tesseract --psm`。**3**=全自动（推荐）；**6**=假定整页均匀文本（有印章/页眉时往往更差）；**4**=单列 |
| `oem` | `3` | `tesseract --oem`。**3**=默认 LSTM |
| `collapse_cjk_spaces` | `true` | OCR 后去掉汉字之间的多余空格 |

### OCR 调参建议（扫描合同）

| 现象 | 可调参数 |
|------|----------|
| 识别比改参前更差、乱码/错字多 | 先确认 `psm: 3`、`pdftoppm_gray: false`、`dpi: 200`（回退默认） |
| 填空【】月/日乱码 | 可试 `dpi: 300`；仍差则人工核对，勿依赖模型补全 |
| 整页顺序乱、漏段 | 试 `psm: 4`；**慎用** `psm: 6`（印章页） |
| 「一字一空」、模型难读 | 保持 `collapse_cjk_spaces: true` |
| 超时 / 很慢 | 降低 `dpi`；加大 `timeout_seconds` |

手工对照：

```powershell
pdftoppm -png -r 200 contract.pdf page
tesseract page-1.png stdout -l chi_sim+eng --psm 3 --oem 3
```

### 扫描件 PDF 说明

若 `pdftotext` 抽不出字，服务会：`pdftoppm -png [-gray] -r <dpi>` → `tesseract --psm/--oem` →（可选）清洗 CJK 空格。  
扫描合同需要 **Poppler + Tesseract（含 chi_sim）**，改 OCR 配置后**重启服务**，并用 `force_reread=true` 强制重抽（否则会命中旧缓存文本）。

纯文字 PDF（`pdftotext` 正常）/ DOCX **不依赖** Tesseract。

### 抽取与 OCR 误差

扫描件 OCR 可能把空白填空误识成数字。`/chat/analyze` 抽取提示已要求：**括号空白/乱码/缺月缺日 → 字段为 `null`，禁止用其它条款补全**。仍建议对关键字段人工复核，或提高 `dpi` 后重试。

### 文件抽取缓存

上传分析时按 **`provider`** 分流（不用 `extract_backend`）：

1. 算 SHA256；读缓存用读锁，强刷/抽取用写锁（`TryLock`，抢不到 →「文档正在识别中」；进程内，无 DB 锁）
2. 未强制且已有可读 txt → 缓存命中
3. `provider=qwen`：上传通义 → 用 file_id 对话；异步写 txt；**新记录成功后再软删旧行**
4. `provider=kimi`：上传并取正文 → 对话 + 落库
5. 其他：本机 OCR → 落库

```yaml
extract:
  timeout_seconds: 180
```
