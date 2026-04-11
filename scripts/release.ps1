param(
  [string]$Version = "dev",
  [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

$bin = Join-Path $OutputDir "aetherd-$Version-windows-amd64.exe"
go build -trimpath -ldflags "-s -w" -o $bin ./cmd/aetherd

$zip = Join-Path $OutputDir "aetherd-$Version-windows-amd64.zip"
if (Test-Path $zip) { Remove-Item $zip -Force }
Compress-Archive -Path $bin -DestinationPath $zip -Force

$hash = Get-FileHash -Algorithm SHA256 $zip
$checksumPath = Join-Path $OutputDir "SHA256SUMS.txt"
"$($hash.Hash.ToLower())  $(Split-Path $zip -Leaf)" | Out-File -Encoding ascii -FilePath $checksumPath

Write-Host "release package: $zip"
Write-Host "checksums: $checksumPath"
