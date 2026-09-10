$ErrorActionPreference = 'Stop'

$skillRoot = Split-Path -Parent $PSScriptRoot
$source = Join-Path $skillRoot 'cmd\feishu-meetings'
$binRoot = Join-Path $skillRoot 'bin'
$targets = @(
    @{ OS = 'windows'; Arch = 'amd64'; Name = 'feishu-meetings.exe' },
    @{ OS = 'windows'; Arch = 'arm64'; Name = 'feishu-meetings.exe' },
    @{ OS = 'linux'; Arch = 'amd64'; Name = 'feishu-meetings' },
    @{ OS = 'linux'; Arch = 'arm64'; Name = 'feishu-meetings' },
    @{ OS = 'darwin'; Arch = 'amd64'; Name = 'feishu-meetings' },
    @{ OS = 'darwin'; Arch = 'arm64'; Name = 'feishu-meetings' }
)

$previousGoEnv = $env:GOENV
$previousCgo = $env:CGO_ENABLED
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
try {
    $env:GOENV = 'off'
    $env:CGO_ENABLED = '0'
    foreach ($target in $targets) {
        $env:GOOS = $target.OS
        $env:GOARCH = $target.Arch
        $targetDir = Join-Path $binRoot "$($target.OS)-$($target.Arch)"
        New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
        $output = Join-Path $targetDir $target.Name
        & go build -trimpath -ldflags '-s -w -buildid=' -o $output $source
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for $($target.OS)-$($target.Arch)"
        }
    }
} finally {
    $env:GOENV = $previousGoEnv
    $env:CGO_ENABLED = $previousCgo
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
}

$checksumLines = Get-ChildItem -Path $binRoot -Recurse -File |
    Where-Object { $_.Name -ne 'SHA256SUMS' } |
    Sort-Object FullName |
    ForEach-Object {
        $relative = [System.IO.Path]::GetRelativePath($skillRoot, $_.FullName).Replace('\', '/')
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
        "$hash  $relative"
    }
[System.IO.File]::WriteAllLines(
    (Join-Path $binRoot 'SHA256SUMS'),
    $checksumLines,
    [System.Text.UTF8Encoding]::new($false)
)

Write-Output "Built $($targets.Count) standalone binaries in $binRoot"
