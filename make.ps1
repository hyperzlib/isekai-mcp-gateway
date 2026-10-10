[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidatePattern('^[a-z0-9]+-[a-z0-9]+$')]
    [string]$Target
)

$ErrorActionPreference = 'Stop'
$Target = $Target.ToLowerInvariant()
$parts = $Target.Split('-')

# Validate the pair against the installed Go toolchain.
$supportedTargets = @(& go tool dist list)
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to list supported Go targets.'
}
if ($supportedTargets -notcontains "$($parts[0])/$($parts[1])") {
    throw "Unsupported Go target: $Target. Use a pair from 'go tool dist list'."
}

$savedEnvironment = @{}
foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

Push-Location -LiteralPath $PSScriptRoot
try {
    $env:GOOS = $parts[0]
    $env:GOARCH = $parts[1]
    $env:CGO_ENABLED = '0'

    $outputDirectory = Join-Path $PSScriptRoot "bin/$Target"
    New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
    $fileName = 'isekai-mcp-gateway'
    if ($env:GOOS -eq 'windows') {
        $fileName += '.exe'
    }
    $outputPath = Join-Path $outputDirectory $fileName

    Write-Host "Building $Target..."
    & go build -o $outputPath ./cmd/gateway
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed with exit code $LASTEXITCODE."
    }
    Write-Host "Built: $outputPath"
}
finally {
    foreach ($name in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
    Pop-Location
}
