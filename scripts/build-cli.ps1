param(
    [string]$Dist = "dist"
)

New-Item -ItemType Directory -Force -Path $Dist | Out-Null

# Compilação multiplataforma das versões linha de comando: Windows e Linux (amd64)
$ErrorActionPreference = "Stop"

# Windows amd64
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o "$Dist/cli-server-windows-amd64.exe" ./cmd/cli-server; go build -o "$Dist/cli-client-windows-amd64.exe" ./cmd/cli-client

# Linux amd64
$env:GOOS = "linux"; $env:GOARCH = "amd64"; go build -o "$Dist/cli-server-linux-amd64" ./cmd/cli-server; go build -o "$Dist/cli-client-linux-amd64" ./cmd/cli-client

Write-Host "Versões CLI compiladas em $Dist"