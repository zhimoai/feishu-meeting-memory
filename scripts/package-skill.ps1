$ErrorActionPreference = 'Stop'

$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$distDir = [System.IO.Path]::GetFullPath((Join-Path $projectRoot 'dist'))
$archivePath = [System.IO.Path]::GetFullPath((Join-Path $distDir 'feishu-meeting-memory-skill.zip'))
$requiredPrefix = $projectRoot.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
if (-not $archivePath.StartsWith($requiredPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Archive path escaped the project directory: $archivePath"
}

$checksumFile = Join-Path $projectRoot 'bin\SHA256SUMS'
$verified = 0
Get-Content -LiteralPath $checksumFile -Encoding UTF8 | ForEach-Object {
    if ($_ -match '^([0-9a-f]{64})  (.+)$') {
        $expected = $Matches[1]
        $binaryPath = [System.IO.Path]::GetFullPath((Join-Path $projectRoot ($Matches[2] -replace '/', '\')))
        if (-not $binaryPath.StartsWith($requiredPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "Checksum target escaped the project directory: $binaryPath"
        }
        if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
            throw "Missing binary: $binaryPath"
        }
        $actual = (Get-FileHash -LiteralPath $binaryPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw "Checksum mismatch: $binaryPath"
        }
        $verified++
    }
}
if ($verified -ne 6) {
    throw "Expected 6 binary checksums, found $verified"
}

[System.IO.Directory]::CreateDirectory($distDir) | Out-Null
if (Test-Path -LiteralPath $archivePath -PathType Leaf) {
    [System.IO.File]::Delete($archivePath)
}
$inputs = @(
    (Join-Path $projectRoot 'SKILL.md'),
    (Join-Path $projectRoot 'agents'),
    (Join-Path $projectRoot 'references'),
    (Join-Path $projectRoot 'scripts'),
    (Join-Path $projectRoot 'bin'),
    (Join-Path $projectRoot 'cmd'),
    (Join-Path $projectRoot 'go.mod')
)
Compress-Archive -LiteralPath $inputs -DestinationPath $archivePath -CompressionLevel Optimal

Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::OpenRead($archivePath)
try {
    $entries = @($archive.Entries | ForEach-Object { $_.FullName.Replace([char]92, [char]47) })
    if ($entries -notcontains 'SKILL.md') {
        throw 'ZIP root does not contain SKILL.md'
    }
    $binaryEntries = @($entries | Where-Object { $_ -match '^bin/(darwin|linux|windows)-(amd64|arm64)/feishu-meetings(\.exe)?$' })
    if ($binaryEntries.Count -ne 6) {
        throw "ZIP contains $($binaryEntries.Count) platform binaries; expected 6"
    }
}
finally {
    $archive.Dispose()
}

$archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
[PSCustomObject]@{
    archive = $archivePath
    size_bytes = (Get-Item -LiteralPath $archivePath).Length
    sha256 = $archiveHash
    verified_binaries = $verified
} | ConvertTo-Json
