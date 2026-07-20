param(
    [string]$Version = $env:STARCAT_VERSION,
    [string]$InstallDir = $env:STARCAT_INSTALL_DIR
)

$ErrorActionPreference = "Stop"
Write-Output "Starcat CLI installer"
Write-Output ""
Write-Output "[1/6] Checking installation environment..."
$Repository = if ($env:STARCAT_GITHUB_REPOSITORY) { $env:STARCAT_GITHUB_REPOSITORY } else { "starcat-app/starcat-cli" }
if (-not $InstallDir) {
    $InstallDir = Join-Path $HOME ".local\bin"
}
Write-Output "[2/6] Resolving release version..."
if (-not $Version) {
    $Headers = @{ Accept = "application/vnd.github+json"; "User-Agent" = "starcat-installer" }
    $Release = Invoke-RestMethod -Headers $Headers -Uri "https://api.github.com/repos/$Repository/releases/latest"
    $Version = $Release.tag_name
}
if (-not $Version) {
    throw "starcat installer: could not determine the latest release version"
}
Write-Output "      Release: $Version"

Write-Output "[3/6] Detecting platform..."
$Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
if ($Architecture -ne "X64") {
    throw "starcat installer: unsupported Windows architecture: $Architecture"
}
Write-Output "      Target: windows/amd64"

$Archive = "starcat_${Version}_windows_amd64.zip"
$ReleaseBase = if ($env:STARCAT_RELEASE_BASE_URL) {
    $env:STARCAT_RELEASE_BASE_URL
} else {
    "https://github.com/$Repository/releases/download/$Version"
}
$TemporaryDir = Join-Path ([System.IO.Path]::GetTempPath()) ("starcat-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $TemporaryDir | Out-Null

try {
    $ArchivePath = Join-Path $TemporaryDir $Archive
    $ChecksumsPath = Join-Path $TemporaryDir "checksums.txt"
    Write-Output "[4/6] Downloading $Archive and checksums.txt..."
    Invoke-WebRequest -Uri "$ReleaseBase/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$ReleaseBase/checksums.txt" -OutFile $ChecksumsPath

    Write-Output "[5/6] Verifying SHA-256 checksum..."
    $EscapedArchive = [regex]::Escape($Archive)
    $ChecksumLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "^([a-fA-F0-9]{64})\s+\*?$EscapedArchive$" } | Select-Object -First 1
    if (-not $ChecksumLine) {
        throw "starcat installer: checksums.txt does not contain $Archive"
    }
    $Expected = ($ChecksumLine -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 $ArchivePath).Hash.ToLowerInvariant()
    if ($Expected -ne $Actual) {
        throw "starcat installer: checksum verification failed for $Archive"
    }
    Write-Output "      Checksum: OK"

    Write-Output "[6/6] Installing Starcat CLI..."
    Expand-Archive -Path $ArchivePath -DestinationPath $TemporaryDir -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $TemporaryDir "starcat.exe") -Destination (Join-Path $InstallDir "starcat.exe") -Force
    Write-Output ""
    Write-Output "✓ Installed Starcat CLI $Version to $(Join-Path $InstallDir 'starcat.exe')"

    $PathEntries = $env:PATH -split ";"
    if ($PathEntries -notcontains $InstallDir) {
        Write-Output ""
        Write-Output "! $InstallDir is not currently in PATH."
        Write-Output "  Add it to PATH before running starcat."
    }

    Write-Output ""
    Write-Output "Get started:"
    Write-Output "  1. Open Starcat > Settings > MCP Service."
    Write-Output "  2. Click Copy Pairing Command, paste it into this terminal, and press Enter."
    Write-Output "  3. Approve the device in Starcat."
    Write-Output "  4. Run: starcat doctor"
    Write-Output ""
    Write-Output "Common commands:"
    Write-Output "  starcat help"
    Write-Output "  starcat capabilities"
    Write-Output "  starcat repo search `"local RAG`""
} finally {
    Remove-Item -Recurse -Force $TemporaryDir -ErrorAction SilentlyContinue
}
