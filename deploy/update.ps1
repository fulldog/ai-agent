# 一键更新：拉取代码、编译后端、编译前端。不重启服务。
#
#   powershell -ExecutionPolicy Bypass -File deploy\update.ps1
#
# 可选环境变量：BIN_PATH（默认 bin\ai-agent.exe）、CGO_ENABLED（默认 0）

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

function Log($msg) {
    Write-Host "[update] $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $msg"
}

function Assert-Exit($step) {
    if ($LASTEXITCODE -ne 0) {
        throw "$step 失败 (exit $LASTEXITCODE)"
    }
}

foreach ($cmd in @("git", "go", "npm")) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        throw "未找到 $cmd"
    }
}

Log "git pull"
git pull
Assert-Exit "git pull"

$BinPath = if ($env:BIN_PATH) { $env:BIN_PATH } else { "bin\ai-agent.exe" }
$binDir = Split-Path $BinPath
if ($binDir) {
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
}
if (-not $env:CGO_ENABLED) {
    $env:CGO_ENABLED = "0"
}
Log "编译后端 → $BinPath"
go build -trimpath -ldflags="-s -w" -o $BinPath .\cmd\server
Assert-Exit "后端编译"

Log "编译前端"
Push-Location web
try {
    if (Test-Path package-lock.json) {
        npm ci
    } else {
        npm install
    }
    Assert-Exit "安装前端依赖"
    npm run build
    Assert-Exit "前端编译"
} finally {
    Pop-Location
}

if (-not (Test-Path (Join-Path $RepoRoot "web\dist\index.html"))) {
    throw "未生成 web\dist\index.html"
}

Log "完成。后端 $BinPath ，前端 web\dist 。服务未重启，需要的话请自行重启。"
