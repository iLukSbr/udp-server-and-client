# Convenience wrapper to build all targets into dist/
# Usage: pwsh .\build.ps1

$ErrorActionPreference = "Stop"

$script = Join-Path -Path $PSScriptRoot -ChildPath "scripts/build-all.ps1"
if (-not (Test-Path $script)) {
  Write-Error "Script not found: $script"
  exit 1
}

& $script @Args
