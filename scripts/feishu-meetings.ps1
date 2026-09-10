$ErrorActionPreference = 'Stop'

$skillRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($env:FEISHU_CONFIG_FILE)) {
    $env:FEISHU_CONFIG_FILE = Join-Path $skillRoot 'config.json'
}
$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
$architecture = switch ($architecture) {
    'x64' { 'amd64' }
    'arm64' { 'arm64' }
    default { throw "Unsupported Windows architecture: $architecture" }
}

$binary = Join-Path $skillRoot "bin\windows-$architecture\feishu-meetings.exe"
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) {
    throw "The standalone Feishu helper is missing: $binary"
}

& $binary @args
exit $LASTEXITCODE
