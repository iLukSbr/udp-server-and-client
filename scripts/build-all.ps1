param(
    [string]$Dist = "dist"
)

$ErrorActionPreference = "Stop"

# Garante que o diretório de saída existe
New-Item -ItemType Directory -Force -Path $Dist | Out-Null

# Preserva configuração original do ambiente para restaurar depois
$origGOOS = $env:GOOS
$origGOARCH = $env:GOARCH

function Build-GUI-Windows {
    Write-Host "Compilando interfaces gráficas (Windows) ..."
    $env:GOOS = "windows"; $env:GOARCH = "amd64"
    go build -o "$Dist/server-windows-amd64.exe" ./cmd/server
    go build -o "$Dist/client-windows-amd64.exe" ./cmd/client
}

function Build-CLI-All {
    Write-Host "Compilando versões CLI (Windows + Linux) ..."
    # Windows
    $env:GOOS = "windows"; $env:GOARCH = "amd64"
    go build -o "$Dist/cli-server-windows-amd64.exe" ./cmd/cli-server
    go build -o "$Dist/cli-client-windows-amd64.exe" ./cmd/cli-client
    # Linux
    $env:GOOS = "linux"; $env:GOARCH = "amd64"
    go build -o "$Dist/cli-server-linux-amd64" ./cmd/cli-server
    go build -o "$Dist/cli-client-linux-amd64" ./cmd/cli-client
}

function Add-Artifact([string]$Path) {
    if (Test-Path $Path) {
        $fi = Get-Item $Path
        $hash = (Get-FileHash -Algorithm SHA256 $Path).Hash.ToLower()
        return [PSCustomObject]@{
            File   = $fi.Name
            Path   = $fi.FullName
            SizeMB = [math]::Round($fi.Length / 1MB, 2)
            SHA256 = $hash
        }
    }
}

try {
    Build-GUI-Windows
    Build-CLI-All

    $list = @()
    $list += Add-Artifact "$Dist/server-windows-amd64.exe"
    $list += Add-Artifact "$Dist/client-windows-amd64.exe"
    $list += Add-Artifact "$Dist/cli-server-windows-amd64.exe"
    $list += Add-Artifact "$Dist/cli-client-windows-amd64.exe"
    $list += Add-Artifact "$Dist/cli-server-linux-amd64"
    $list += Add-Artifact "$Dist/cli-client-linux-amd64"

    Write-Host "`nArtefatos compilados em $Dist:" -ForegroundColor Cyan
    $list | Where-Object { $_ } | Format-Table -AutoSize

    Write-Host "`nObservações:" -ForegroundColor Yellow
    Write-Host "- Binários Linux CLI não têm extensão; no Linux execute: chmod +x ./dist/cli-*-linux-amd64"
    Write-Host "- Interfaces gráficas Linux não são produzidas aqui; use CLI para demonstrações multiplataforma."
}
finally {
    # Restaura configuração original do ambiente
    $env:GOOS = $origGOOS
    $env:GOARCH = $origGOARCH
}
