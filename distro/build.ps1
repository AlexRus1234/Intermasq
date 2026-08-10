$ErrorActionPreference = 'Stop'

$DistroDir = (Resolve-Path (Join-Path $PSScriptRoot '.')).Path
$RepoDir = (Resolve-Path (Join-Path $DistroDir '..')).Path
$ImageName = if ($env:INTERMASQ_LAB_BUILDER_IMAGE) { $env:INTERMASQ_LAB_BUILDER_IMAGE } else { 'intermasq-lab-builder:local' }

if (-not (Get-Command podman -ErrorAction SilentlyContinue)) {
    throw 'podman is required'
}

function Invoke-PodmanQuiet {
    param([string[]]$Arguments)

    $PreviousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & podman @Arguments 2>&1 | Out-Null
    $ExitCode = $LASTEXITCODE
    $ErrorActionPreference = $PreviousErrorActionPreference
    return $ExitCode
}

$MachineRows = @(podman machine list --format "{{.Name}}`t{{.VMType}}" 2>$null)
$DefaultMachine = @($MachineRows | Where-Object {
    $ListedName = ($_ -split "`t", 2)[0]
    $ListedName.TrimEnd('*') -ne $ListedName
})
$WindowsHost = ($env:OS -eq 'Windows_NT')
if ($WindowsHost -and $DefaultMachine.Count -gt 0) {
    $DefaultProvider = ($DefaultMachine[0] -split "`t", 2)[1].Trim()
    if ($DefaultProvider -ne 'wsl') {
        throw "The default Podman machine uses '$DefaultProvider'. Windows builds require a WSL provider; remove or recreate the default machine with 'podman machine init --provider wsl'."
    }
}
$MachineName = if ($DefaultMachine.Count -gt 0) {
    ($DefaultMachine[0] -split "`t", 2)[0].TrimEnd('*')
} else {
    'podman-machine-default'
}

$InfoExitCode = Invoke-PodmanQuiet @('--connection', $MachineName, 'info')
$EngineReady = ($InfoExitCode -eq 0)
if (-not $EngineReady) {
    $MachineRows = @(podman machine list --format "{{.Name}}`t{{.VMType}}" 2>$null)
    if ($MachineRows.Count -eq 0) {
        if (Get-Command wsl.exe -ErrorAction SilentlyContinue) {
            podman machine init --provider wsl --now
        } else {
            podman machine init --now
        }
    } else {
        $StartExitCode = Invoke-PodmanQuiet @('machine', 'start', $MachineName)
        if ($StartExitCode -ne 0) {
            throw "Podman default machine '$MachineName' could not be started"
        }
    }
}

$OutputDir = Join-Path $DistroDir 'output'
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

podman --connection $MachineName build --pull=missing -f (Join-Path $DistroDir 'Containerfile') -t $ImageName $DistroDir
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$RepoMount = "${RepoDir}:/src:ro"
$OutputMount = "${OutputDir}:/out:rw"
podman --connection $MachineName run --rm --user 0 -v $RepoMount -v $OutputMount $ImageName /bin/sh /src/distro/build-inside.sh
exit $LASTEXITCODE
