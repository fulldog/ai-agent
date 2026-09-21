# 裸机部署（Windows）：编译 server / web，重启进程，生成 Nginx 配置（IP:端口）。
#
#   powershell -ExecutionPolicy Bypass -File deploy\bare\deploy.ps1
#
# 环境变量（可选）同 Linux 脚本：WEB_PORT、API_PORT、SKIP_SERVER、SKIP_WEB、SKIP_NGINX、NGINX_CONF

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $RepoRoot

function Log($msg) { Write-Host "[ai-agent-bare] $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $msg" }

$Config = if ($env:CONFIG) { $env:CONFIG } else { "configs\config.yaml" }
$BinPath = if ($env:BIN_PATH) { $env:BIN_PATH } else { "bin\ai-agent.exe" }
$PidFile = if ($env:PID_FILE) { $env:PID_FILE } else { "data\ai-agent.pid" }
$StdoutLog = if ($env:STDOUT_LOG) { $env:STDOUT_LOG } else { "data\logs\server.stdout.log" }
$WebPort = if ($env:WEB_PORT) { $env:WEB_PORT } else { "8080" }
$ApiHost = if ($env:API_HOST) { $env:API_HOST } else { "127.0.0.1" }
$Template = Join-Path $PSScriptRoot "nginx.conf.template"
$SkipServer = $env:SKIP_SERVER -eq "1"
$SkipWeb = $env:SKIP_WEB -eq "1"
$SkipNginx = $env:SKIP_NGINX -eq "1"

function Get-ApiPort {
    if ($env:API_PORT) { return $env:API_PORT }
    if (Test-Path $Config) {
        $line = Select-String -Path $Config -Pattern '^\s*addr:' | Select-Object -First 1
        if ($line -and $line.Line -match ':(\d+)\s*$') { return $Matches[1] }
    }
    return "18090"
}

$ApiPort = Get-ApiPort
$ApiUpstream = "${ApiHost}:${ApiPort}"
$WebRoot = (Join-Path $RepoRoot "web\dist") -replace '\\', '/'

if (-not $SkipServer) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw "未找到 go" }
    New-Item -ItemType Directory -Force -Path (Split-Path $BinPath) | Out-Null
    Log "编译 server → $BinPath"
    $env:CGO_ENABLED = if ($env:CGO_ENABLED) { $env:CGO_ENABLED } else { "0" }
    go build -trimpath -ldflags="-s -w" -o $BinPath .\cmd\server
    Log "server 编译完成"
}

if (-not $SkipWeb) {
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) { throw "未找到 npm" }
    Log "编译 web"
    Push-Location web
    try {
        if (Test-Path package-lock.json) { npm ci } else { npm install }
        npm run build
    } finally {
        Pop-Location
    }
    if (-not (Test-Path (Join-Path $RepoRoot "web\dist\index.html"))) {
        throw "未生成 web\dist\index.html"
    }
}

if (-not $SkipServer) {
    if (-not (Test-Path $Config)) { throw "缺少 $Config" }
    New-Item -ItemType Directory -Force -Path data\logs, data\attachments | Out-Null

    if (Test-Path $PidFile) {
        $old = (Get-Content $PidFile -ErrorAction SilentlyContinue | Select-Object -First 1).Trim()
        if ($old -match '^\d+$') {
            $p = Get-Process -Id ([int]$old) -ErrorAction SilentlyContinue
            if ($p) {
                Log "停止 PID $old"
                Stop-Process -Id ([int]$old) -Force -ErrorAction SilentlyContinue
            }
        }
        Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
    }

    Get-NetTCPConnection -LocalPort ([int]$ApiPort) -State Listen -ErrorAction SilentlyContinue |
        ForEach-Object {
            Log "释放端口 $ApiPort，结束 PID $($_.OwningProcess)"
            Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue
        }

    $binAbs = (Resolve-Path $BinPath).Path
    $cfgAbs = (Resolve-Path $Config).Path
    New-Item -ItemType Directory -Force -Path (Split-Path $StdoutLog) | Out-Null
    $errLog = "$StdoutLog.err"
    Log "启动 $binAbs -config $cfgAbs"
    $proc = Start-Process -FilePath $binAbs -ArgumentList "-config", $cfgAbs -RedirectStandardOutput $StdoutLog -RedirectStandardError $errLog -WindowStyle Hidden -PassThru
    Set-Content -Path $PidFile -Value $proc.Id -NoNewline
    Log "已写入 $PidFile pid=$($proc.Id)"

    $ok = $false
    for ($i = 0; $i -lt 40; $i++) {
        try {
            Invoke-WebRequest -UseBasicParsing "http://${ApiHost}:${ApiPort}/health" | Out-Null
            $ok = $true
            break
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $ok) {
        if (Test-Path $StdoutLog) { Get-Content $StdoutLog -Tail 80 }
        throw "后端未就绪 http://${ApiHost}:${ApiPort}/health"
    }
    Log "健康检查通过"
}

if (-not $SkipNginx) {
    if (-not (Test-Path $Template)) { throw "缺少 $Template" }
    $rendered = (Get-Content $Template -Raw) `
        -replace '__WEB_PORT__', $WebPort `
        -replace '__WEB_ROOT__', $WebRoot `
        -replace '__API_UPSTREAM__', $ApiUpstream
    $dest = if ($env:NGINX_CONF) { $env:NGINX_CONF } else { Join-Path $PSScriptRoot "ai-agent-web.conf" }
    Set-Content -Path $dest -Value $rendered -Encoding utf8
    Log "已生成 Nginx 配置: $dest（listen $WebPort, server_name _）"
    $nginx = Get-Command nginx -ErrorAction SilentlyContinue
    if ($nginx) {
        $installTo = $env:NGINX_CONF
        if (-not $installTo) {
            Log "未设置 NGINX_CONF，请把 $dest 复制到 Nginx conf.d 后执行 nginx -s reload"
        } else {
            nginx -t
            nginx -s reload
            Log "nginx reload 完成"
        }
    } else {
        Log "本机无 nginx 命令：把 $dest 拷到 Linux /etc/nginx/conf.d/ 后 nginx -t && nginx -s reload"
    }
}

Log "完成。控制台（无域名）: http://<本机IP>:${WebPort}/"
Log "API Base 留空，走 Nginx /api 反代；连接设置里填写 X-API-Key"
