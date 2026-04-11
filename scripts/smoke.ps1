param(
  [string]$Binary = ".\\dist\\aetherd.exe"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Binary)) {
  throw "binary not found: $Binary (run scripts/build.ps1 first)"
}

$root = Join-Path $env:TEMP ("aether-smoke-" + [guid]::NewGuid().ToString("N"))
$aDir = Join-Path $root "a"
$bDir = Join-Path $root "b"
New-Item -ItemType Directory -Force -Path $aDir, $bDir | Out-Null

$envA = @{
  AETHER_LISTEN_ADDR = "127.0.0.1:19101"
  AETHER_ADVERTISE_ADDR = "127.0.0.1:19101"
  AETHER_DATA_DIR = $aDir
  AETHER_DEV_CLEARNET = "true"
  AETHER_REQUIRE_TOR = "false"
  AETHER_LOG_LEVEL = "warn"
}
$envB = @{
  AETHER_LISTEN_ADDR = "127.0.0.1:19102"
  AETHER_ADVERTISE_ADDR = "127.0.0.1:19102"
  AETHER_DATA_DIR = $bDir
  AETHER_BOOTSTRAP_PEERS = "127.0.0.1:19101"
  AETHER_DEV_CLEARNET = "true"
  AETHER_REQUIRE_TOR = "false"
  AETHER_LOG_LEVEL = "warn"
}

function Start-Node([hashtable]$EnvVars) {
  $psi = New-Object System.Diagnostics.ProcessStartInfo
  $psi.FileName = $Binary
  $psi.Arguments = "serve"
  $psi.UseShellExecute = $false
  $psi.RedirectStandardOutput = $true
  $psi.RedirectStandardError = $true
  foreach ($k in $EnvVars.Keys) {
    $psi.Environment[$k] = $EnvVars[$k]
  }
  return [System.Diagnostics.Process]::Start($psi)
}

$procA = Start-Node $envA
$procB = Start-Node $envB
try {
  Start-Sleep -Seconds 2
  $postEnv = @{
    AETHER_DATA_DIR = $aDir
    AETHER_DEV_CLEARNET = "true"
    AETHER_REQUIRE_TOR = "false"
    AETHER_BOOTSTRAP_PEERS = "127.0.0.1:19102"
    AETHER_LOG_LEVEL = "warn"
  }
  foreach ($k in $postEnv.Keys) { $env:$k = $postEnv[$k] }
  & $Binary post "smoke-test-message" | Out-Null
  Write-Host "smoke test sent a message successfully"
}
finally {
  if ($procA -and -not $procA.HasExited) { $procA.Kill() }
  if ($procB -and -not $procB.HasExited) { $procB.Kill() }
}
