# Nudge: desinstalador por usuario para Windows 10/11.
#
# Uso (menú Inicio > Nudge > Uninstall Nudge, o "Uninstall Nudge.cmd"):
#   uninstall.ps1 [-RemoveData ask|yes|no] [-NoPause]
#
# Qué hace:
#   1. Detiene Nudge.
#   2. Borra los accesos directos (menú Inicio e inicio automático).
#   3. Quita la carpeta del PATH del usuario.
#   4. Quita las reglas del firewall "Nudge" y "Nudge mDNS" si existen
#      (pide permiso de administrador).
#   5. Borra %LOCALAPPDATA%\Programs\Nudge.
#   6. Pregunta antes de borrar los datos (%APPDATA%\nudge, por defecto No):
#      ahí están los teléfonos emparejados y el log.
#
# Nunca borra nada fuera de esas rutas. Compatible con Windows PowerShell 5.1.
[CmdletBinding()]
param(
    [ValidateSet('ask', 'yes', 'no')]
    [string]$RemoveData = 'ask',
    [switch]$NoPause
)

$ErrorActionPreference = 'Stop'

function Wait-Close {
    if ($NoPause) { return }
    try { [void](Read-Host 'Presioná Enter para cerrar') } catch { }
}

function Test-Yes([string]$Answer) {
    if ($null -eq $Answer) { return $false }
    return ($Answer.Trim() -match '^(s|si|s\u00ed|y|yes)$')
}

function Test-SamePath([string]$A, [string]$B) {
    if ([string]::IsNullOrEmpty($A) -or [string]::IsNullOrEmpty($B)) { return $false }
    return [string]::Equals($A.Trim().TrimEnd('\'), $B.Trim().TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)
}

function Invoke-NetshExitCode([string]$Arguments) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = Join-Path $env:SystemRoot 'System32\netsh.exe'
    $psi.Arguments = $Arguments
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    [void]$p.StandardOutput.ReadToEnd()
    [void]$p.StandardError.ReadToEnd()
    $p.WaitForExit()
    return $p.ExitCode
}

function Remove-IfExists([string]$Path) {
    if (Test-Path -LiteralPath $Path) { Remove-Item -LiteralPath $Path -Force }
}

$exitCode = 0
try {
    Write-Host '== Desinstalando Nudge ==' -ForegroundColor Cyan

    if ([string]::IsNullOrEmpty($env:LOCALAPPDATA)) { throw 'No se encontró la variable LOCALAPPDATA.' }
    if ([string]::IsNullOrEmpty($env:APPDATA)) { throw 'No se encontró la variable APPDATA.' }
    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\Nudge'
    $dataDir = Join-Path $env:APPDATA 'nudge'

    # Salir de la carpeta de instalación: Windows no deja borrar la carpeta
    # actual de un proceso.
    if (-not [string]::IsNullOrEmpty($env:USERPROFILE)) {
        Set-Location -LiteralPath $env:USERPROFILE
        [Environment]::CurrentDirectory = $env:USERPROFILE
    }

    # 1. Detener Nudge (cualquier nudge-background.exe y el nudge.exe instalado).
    $procs = @(Get-Process -Name 'nudge-background' -ErrorAction SilentlyContinue)
    foreach ($p in @(Get-Process -Name 'nudge' -ErrorAction SilentlyContinue)) {
        $path = $null
        try { $path = $p.Path } catch { }
        if ($path -and $path.StartsWith($installDir + '\', [StringComparison]::OrdinalIgnoreCase)) { $procs += $p }
    }
    if ($procs.Count -gt 0) {
        Write-Host 'Deteniendo Nudge...'
        foreach ($p in $procs) { try { Stop-Process -Id $p.Id -Force -ErrorAction Stop } catch { } }
        foreach ($p in $procs) { try { Wait-Process -Id $p.Id -Timeout 10 -ErrorAction SilentlyContinue } catch { } }
        Start-Sleep -Milliseconds 500
    }

    # 2. Accesos directos: solo los que crea el instalador.
    $startup = [Environment]::GetFolderPath('Startup')
    if (-not [string]::IsNullOrEmpty($startup)) { Remove-IfExists (Join-Path $startup 'Nudge.lnk') }
    $programs = [Environment]::GetFolderPath('Programs')
    if (-not [string]::IsNullOrEmpty($programs)) {
        $menuDir = Join-Path $programs 'Nudge'
        foreach ($n in @('Nudge.lnk', 'Nudge - autostart on-off.lnk', 'Uninstall Nudge.lnk')) {
            Remove-IfExists (Join-Path $menuDir $n)
        }
        # Borrar la carpeta del menú solo si quedó vacía.
        if ((Test-Path -LiteralPath $menuDir) -and -not (Get-ChildItem -LiteralPath $menuDir -Force)) {
            Remove-Item -LiteralPath $menuDir -Force
        }
    }
    Write-Host 'Accesos directos borrados.'

    # 3. PATH del usuario: quitar solo la entrada de Nudge.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not [string]::IsNullOrEmpty($userPath)) {
        $kept = @()
        $removed = $false
        foreach ($entry in $userPath.Split(';')) {
            if (Test-SamePath ([Environment]::ExpandEnvironmentVariables($entry)) $installDir) {
                $removed = $true
            } elseif ($entry -ne '') {
                $kept += $entry
            }
        }
        if ($removed) {
            [Environment]::SetEnvironmentVariable('Path', ($kept -join ';'), 'User')
            Write-Host 'Carpeta quitada del PATH del usuario.'
        }
    }

    # 4. Firewall: solo si las reglas existen (pide administrador).
    $tcpRule = ((Invoke-NetshExitCode 'advfirewall firewall show rule name="Nudge"') -eq 0)
    $mdnsRule = ((Invoke-NetshExitCode 'advfirewall firewall show rule name="Nudge mDNS"') -eq 0)
    $hasRule = ($tcpRule -or $mdnsRule)
    if ($hasRule) {
        Write-Host 'Quitando las reglas del firewall (acepta el aviso de administrador)...'
        $fwScript = Join-Path $installDir 'firewall.ps1'
        if (-not (Test-Path -LiteralPath $fwScript)) { $fwScript = Join-Path $PSScriptRoot 'firewall.ps1' }
        $psExe = Join-Path $PSHOME 'powershell.exe'
        $fwArgs = '-NoProfile -ExecutionPolicy Bypass -File "{0}" -Action remove' -f $fwScript
        try {
            $proc = Start-Process -FilePath $psExe -ArgumentList $fwArgs -Verb RunAs -WindowStyle Hidden -Wait -PassThru
            if ($proc.ExitCode -eq 0) {
                Write-Host 'Reglas del firewall borradas.'
            } else {
                Write-Host "No se pudieron borrar las reglas del firewall (código $($proc.ExitCode))." -ForegroundColor Yellow
            }
        } catch {
            Write-Host 'Reglas del firewall no borradas (se canceló el permiso de administrador).' -ForegroundColor Yellow
            Write-Host 'Para borrarlas a mano, en una consola de administrador:'
            Write-Host '  netsh advfirewall firewall delete rule name="Nudge"'
            Write-Host '  netsh advfirewall firewall delete rule name="Nudge mDNS"'
        }
    }

    # 5. Carpeta de instalación (solo esta ruta exacta).
    if (Test-Path -LiteralPath $installDir) {
        try {
            Remove-Item -LiteralPath $installDir -Recurse -Force
            Write-Host "Borrado $installDir"
        } catch {
            Write-Host "No se pudo borrar $installDir ($($_.Exception.Message)). Borrala a mano." -ForegroundColor Yellow
        }
    }

    # 6. Datos del usuario (teléfonos emparejados, log): preguntar, por defecto No.
    if (Test-Path -LiteralPath $dataDir) {
        $wipe = $false
        if ($RemoveData -eq 'yes') {
            $wipe = $true
        } elseif ($RemoveData -eq 'ask') {
            $wipe = Test-Yes (Read-Host "¿Borrar también los datos en $dataDir (teléfonos emparejados y log)? (s/N)")
        }
        if ($wipe) {
            Remove-Item -LiteralPath $dataDir -Recurse -Force
            Write-Host "Borrado $dataDir"
        } else {
            Write-Host "Se conservan los datos en $dataDir"
        }
    }

    Write-Host 'Nudge desinstalado.' -ForegroundColor Green
}
catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    $exitCode = 1
}

Wait-Close
exit $exitCode
