param(
    [string]$Version = $env:STARCAT_VERSION,
    [string]$InstallDir = $env:STARCAT_INSTALL_DIR
)

$ErrorActionPreference = "Stop"
$Repository = if ($env:STARCAT_GITHUB_REPOSITORY) { $env:STARCAT_GITHUB_REPOSITORY } else { "dong4j/starcat-cli" }
if (-not $InstallDir) {
    $InstallDir = Join-Path $HOME ".local\bin"
}
if (-not $Version) {
    $Headers = @{ Accept = "application/vnd.github+json"; "User-Agent" = "starcat-installer" }
    $Release = Invoke-RestMethod -Headers $Headers -Uri "https://api.github.com/repos/$Repository/releases/latest"
    $Version = $Release.tag_name
}
if (-not $Version) {
    throw "starcat installer: could not determine the latest release version"
}

$Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
if ($Architecture -ne "X64") {
    throw "starcat installer: unsupported Windows architecture: $Architecture"
}

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
    Invoke-WebRequest -Uri "$ReleaseBase/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$ReleaseBase/checksums.txt" -OutFile $ChecksumsPath

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

    Expand-Archive -Path $ArchivePath -DestinationPath $TemporaryDir -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $TemporaryDir "starcat.exe") -Destination (Join-Path $InstallDir "starcat.exe") -Force
    Write-Output "Installed Starcat CLI $Version to $(Join-Path $InstallDir 'starcat.exe')"

    $PathEntries = $env:PATH -split ";"
    if ($PathEntries -notcontains $InstallDir) {
        Write-Output "Add $InstallDir to PATH before running starcat."
    }
} finally {
    Remove-Item -Recurse -Force $TemporaryDir -ErrorAction SilentlyContinue
}
