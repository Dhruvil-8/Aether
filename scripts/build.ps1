param(
  [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
go build -o (Join-Path $OutputDir "aetherd.exe") ./cmd/aetherd
Write-Host "built $(Join-Path $OutputDir 'aetherd.exe')"
