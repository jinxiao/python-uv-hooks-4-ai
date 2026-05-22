$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Repo = if ($env:UV_PYTHON_HOOK_REPO) { $env:UV_PYTHON_HOOK_REPO } else { "jinxiao/python-uv-hooks-4-ai" }
$InstallDir = if ($env:UV_PYTHON_HOOK_INSTALL_DIR) { $env:UV_PYTHON_HOOK_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$NoModifyPath = $env:UV_PYTHON_HOOK_NO_MODIFY_PATH -eq "1"

$archEnv = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
switch ($archEnv) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    default { throw "unsupported architecture: $archEnv" }
}

$ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"
$Headers = @{ "User-Agent" = "uv-python-hook-installer" }
$Release = Invoke-RestMethod -Uri $ApiUrl -Headers $Headers

if (-not $Release.tag_name) {
    throw "could not determine latest release tag from $ApiUrl"
}

$Version = $Release.tag_name -replace "^v", ""
$AssetName = "uv-python-hook_${Version}_windows_${Arch}.zip"
$Asset = $Release.assets | Where-Object { $_.name -eq $AssetName } | Select-Object -First 1
$Checksums = $Release.assets | Where-Object { $_.name -eq "checksums.txt" } | Select-Object -First 1

if (-not $Asset) {
    throw "release $($Release.tag_name) does not contain $AssetName"
}
if (-not $Checksums) {
    throw "release $($Release.tag_name) does not contain checksums.txt"
}

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("uv-python-hook-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null

try {
    $ArchivePath = Join-Path $TempDir $AssetName
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"
    Invoke-WebRequest -Uri $Asset.browser_download_url -Headers $Headers -OutFile $ArchivePath
    Invoke-WebRequest -Uri $Checksums.browser_download_url -Headers $Headers -OutFile $ChecksumsPath

    $Line = Get-Content -Path $ChecksumsPath | Where-Object { $_ -match "\s$([Regex]::Escape($AssetName))$" } | Select-Object -First 1
    if (-not $Line) {
        throw "checksums.txt does not contain $AssetName"
    }

    $Expected = ($Line -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    if ($Expected -ne $Actual) {
        throw "checksum mismatch for $AssetName"
    }

    Expand-Archive -LiteralPath $ArchivePath -DestinationPath $TempDir -Force
    $BinaryPath = Join-Path $TempDir "uv-python-hook.exe"
    if (-not (Test-Path -LiteralPath $BinaryPath)) {
        throw "archive did not contain uv-python-hook.exe"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $Destination = Join-Path $InstallDir "uv-python-hook.exe"
    Copy-Item -LiteralPath $BinaryPath -Destination $Destination -Force

    if (-not $NoModifyPath) {
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $Parts = @()
        if ($UserPath) {
            $Parts = $UserPath -split ";" | Where-Object { $_ }
        }
        $AlreadyPresent = $Parts | Where-Object { $_.TrimEnd("\") -ieq $InstallDir.TrimEnd("\") } | Select-Object -First 1
        if (-not $AlreadyPresent) {
            $NewUserPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
            [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
            Write-Host "Updated user Path with $InstallDir"
        }
        if (($env:Path -split ";") -notcontains $InstallDir) {
            $env:Path = "$InstallDir;$env:Path"
        }
    }

    Write-Host "Installed uv-python-hook $Version to $Destination"
} finally {
    Remove-Item -LiteralPath $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
