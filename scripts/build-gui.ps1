param(
    [string]$Dist = "dist"
)

New-Item -ItemType Directory -Force -Path $Dist | Out-Null

$ErrorActionPreference = "Stop"

# Compilação das interfaces gráficas apenas para Windows (Fyne requer toolchain nativa da plataforma).
# Para Linux seria necessário toolchain Linux com bibliotecas gráficas. Use CLI para testes multiplataforma.
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o "$Dist/server-windows-amd64.exe" ./cmd/server; go build -o "$Dist/client-windows-amd64.exe" ./cmd/client

Write-Host "Interfaces gráficas compiladas em $Dist"