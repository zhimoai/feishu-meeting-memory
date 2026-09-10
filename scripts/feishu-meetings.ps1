$ErrorActionPreference = 'Stop'

$skillRoot = Split-Path -Parent $PSScriptRoot
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

