param(
    [string]$AppId,
    [string]$ConfigPath = (Join-Path $env:APPDATA 'feishu-meeting-memory\config.json')
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($AppId)) {
    $AppId = Read-Host 'Feishu App ID'
}
if ($AppId -notmatch '^cli_[A-Za-z0-9_-]+$') {
    throw 'App ID should start with cli_'
}
$secureSecret = Read-Host 'Feishu App Secret (input hidden)' -AsSecureString
$secretPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureSecret)
try {
    $appSecret = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($secretPointer)
    if ($appSecret -notmatch '^[A-Za-z0-9_-]{16,200}$') {
        throw 'App Secret format is invalid'
    }
    $absolutePath = [System.IO.Path]::GetFullPath($ConfigPath)
    [System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($absolutePath)) | Out-Null
    $payload = [ordered]@{
        app_id = $AppId
        app_secret = $appSecret
        oauth_redirect_uri = 'http://127.0.0.1:8080/callback'
        oauth_scope = 'space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access'
        api_base = 'https://open.feishu.cn'
        http_timeout_seconds = 30
    } | ConvertTo-Json
    [System.IO.File]::WriteAllText($absolutePath, $payload, [System.Text.UTF8Encoding]::new($false))

    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $security = [Security.AccessControl.FileSecurity]::new()
    $security.SetAccessRuleProtection($true, $false)
    $security.SetOwner([Security.Principal.NTAccount]::new($currentUser))
    $rule = [Security.AccessControl.FileSystemAccessRule]::new(
        $currentUser,
        [Security.AccessControl.FileSystemRights]::FullControl,
        [Security.AccessControl.AccessControlType]::Allow
    )
    $security.AddAccessRule($rule)
    Set-Acl -LiteralPath $absolutePath -AclObject $security

    [PSCustomObject]@{
        configured = $true
        config_file = $absolutePath
        app_id = $AppId
        secret_printed = $false
        next_step = 'Run scripts/feishu-meetings.ps1 oauth-login --full and sign in with the current user account'
    } | ConvertTo-Json
}
finally {
    if ($secretPointer -ne [IntPtr]::Zero) {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($secretPointer)
    }
}
