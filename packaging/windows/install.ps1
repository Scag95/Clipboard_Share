# Nudge: instalador por usuario para Windows 10/11 (no requiere administrador).
#
# Uso (normalmente con doble clic en "Install Nudge.cmd"):
#   install.ps1 [-NoAutostart] [-FirewallRule ask|yes|no] [-NoStart] [-NoPause]
#
# Qué hace (se puede ejecutar de nuevo: actualiza en el lugar):
#   1. Detiene nudge-background.exe si está corriendo.
#   2. Copia los archivos a %LOCALAPPDATA%\Programs\Nudge.
#   3. Crea los accesos directos del menú Inicio (carpeta "Nudge").
#   4. Activa el inicio automático (salvo -NoAutostart, que lo desactiva).
#   5. Agrega la carpeta al PATH del usuario (para usar "nudge pair").
#   6. Ofrece agregar las reglas del firewall (paso con administrador).
#   7. Inicia nudge-background.exe (salvo -NoStart).
#
# Nunca borra nada fuera de su carpeta, sus accesos directos y sus reglas.
# Compatible con Windows PowerShell 5.1.
[CmdletBinding()]
param(
    [switch]$NoAutostart,
    [ValidateSet('ask', 'yes', 'no')]
    [string]$FirewallRule = 'ask',
    [switch]$NoStart,
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

# Compara dos rutas sin distinguir mayúsculas ni barra final.
function Test-SamePath([string]$A, [string]$B) {
    if ([string]::IsNullOrEmpty($A) -or [string]::IsNullOrEmpty($B)) { return $false }
    return [string]::Equals($A.Trim().TrimEnd('\'), $B.Trim().TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)
}

function New-Shortcut([string]$Path, [string]$Target, [string]$Arguments, [string]$WorkDir, [string]$Icon, [string]$Description) {
    $shell = New-Object -ComObject WScript.Shell
    $lnk = $shell.CreateShortcut($Path)
    $lnk.TargetPath = $Target
    $lnk.Arguments = $Arguments
    $lnk.WorkingDirectory = $WorkDir
    if (-not [string]::IsNullOrEmpty($Icon)) { $lnk.IconLocation = "$Icon,0" }
    $lnk.Description = $Description
    $lnk.Save()
}

# Ejecuta netsh.exe con la línea de argumentos exacta; devuelve el código de salida.
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

# Detiene los procesos de Nudge que bloquearían la copia de archivos.
function Stop-Nudge([string]$InstallDir) {
    $procs = @()
    # Cualquier nudge-background.exe: usa el puerto 8765 y se reinicia al final.
    $procs += @(Get-Process -Name 'nudge-background' -ErrorAction SilentlyContinue)
    # nudge.exe solo si es el de la carpeta de instalación.
    foreach ($p in @(Get-Process -Name 'nudge' -ErrorAction SilentlyContinue)) {
        $path = $null
        try { $path = $p.Path } catch { }
        if ($path -and $path.StartsWith($InstallDir + '\', [StringComparison]::OrdinalIgnoreCase)) {
            $procs += $p
        }
    }
    if ($procs.Count -eq 0) { return }
    Write-Host 'Deteniendo Nudge...'
    foreach ($p in $procs) {
        try { Stop-Process -Id $p.Id -Force -ErrorAction Stop } catch { }
    }
    foreach ($p in $procs) {
        try { Wait-Process -Id $p.Id -Timeout 10 -ErrorAction SilentlyContinue } catch { }
    }
    Start-Sleep -Milliseconds 500
}

$exitCode = 0
try {
    Write-Host '== Instalando Nudge ==' -ForegroundColor Cyan
    Write-Host 'Aviso: esta versión para Windows no está probada todavía.'

    if ([string]::IsNullOrEmpty($env:LOCALAPPDATA)) { throw 'No se encontró la variable LOCALAPPDATA.' }
    $srcDir = $PSScriptRoot
    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\Nudge'
    $bgExe = Join-Path $installDir 'nudge-background.exe'
    $icon = Join-Path $installDir 'nudge.ico'

    # 0. Comprobar que el zip se extrajo completo.
    $files = @('nudge.exe', 'nudge-background.exe', 'nudge.ico', 'uninstall.ps1',
        'toggle-autostart.ps1', 'firewall.ps1', 'README-windows.txt')
    foreach ($f in $files) {
        if (-not (Test-Path -LiteralPath (Join-Path $srcDir $f))) {
            throw "Falta '$f' junto a install.ps1. Extraé el zip completo (clic derecho > Extraer todo) y volvé a ejecutar 'Install Nudge.cmd'."
        }
    }
    $sameDir = Test-SamePath $srcDir $installDir

    # 1. Detener Nudge.
    Stop-Nudge $installDir

    # 2. Copiar archivos.
    if (-not $sameDir) {
        Write-Host "Copiando archivos a $installDir"
        if (-not (Test-Path -LiteralPath $installDir)) {
            [void](New-Item -ItemType Directory -Path $installDir -Force)
        }
        foreach ($f in $files) {
            Copy-Item -LiteralPath (Join-Path $srcDir $f) -Destination (Join-Path $installDir $f) -Force
        }
    }
    # Quitar la marca "descargado de Internet" de los archivos instalados,
    # para que el inicio automático no muestre una advertencia en cada sesión.
    foreach ($f in $files) {
        try { Unblock-File -LiteralPath (Join-Path $installDir $f) -ErrorAction Stop } catch { }
    }

    # 3. Menú Inicio.
    $programs = [Environment]::GetFolderPath('Programs')
    if ([string]::IsNullOrEmpty($programs)) { throw 'No se encontró la carpeta del menú Inicio.' }
    $menuDir = Join-Path $programs 'Nudge'
    if (-not (Test-Path -LiteralPath $menuDir)) {
        [void](New-Item -ItemType Directory -Path $menuDir -Force)
    }
    $psExe = Join-Path $PSHOME 'powershell.exe'
    $toggleArgs = '-NoProfile -ExecutionPolicy Bypass -File "{0}" toggle' -f (Join-Path $installDir 'toggle-autostart.ps1')
    $uninstallArgs = '-NoProfile -ExecutionPolicy Bypass -File "{0}"' -f (Join-Path $installDir 'uninstall.ps1')
    New-Shortcut (Join-Path $menuDir 'Nudge.lnk') $bgExe '' $installDir $icon 'Iniciar Nudge'
    New-Shortcut (Join-Path $menuDir 'Nudge - autostart on-off.lnk') $psExe $toggleArgs $installDir $icon 'Activar o desactivar el inicio automático de Nudge'
    # Carpeta de trabajo fuera de la instalación, para poder borrarla.
    New-Shortcut (Join-Path $menuDir 'Uninstall Nudge.lnk') $psExe $uninstallArgs $env:USERPROFILE $icon 'Desinstalar Nudge'
    Write-Host 'Accesos directos creados en el menú Inicio (carpeta Nudge).'

    # 4. Inicio automático (reutiliza toggle-autostart.ps1).
    $toggle = Join-Path $installDir 'toggle-autostart.ps1'
    if ($NoAutostart) { $autoState = 'off' } else { $autoState = 'on' }
    & $toggle -State $autoState -NoPause
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo configurar el inicio automático.' }

    # 5. PATH del usuario: agregar la carpeta una sola vez, sin tocar el resto.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $present = $false
    if (-not [string]::IsNullOrEmpty($userPath)) {
        foreach ($entry in $userPath.Split(';')) {
            if (Test-SamePath ([Environment]::ExpandEnvironmentVariables($entry)) $installDir) { $present = $true }
        }
    }
    if ($present) {
        Write-Host 'La carpeta ya está en el PATH del usuario.'
    } else {
        if ([string]::IsNullOrEmpty($userPath)) {
            $newPath = $installDir
        } else {
            $newPath = $userPath.TrimEnd(';') + ';' + $installDir
        }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host 'Carpeta agregada al PATH del usuario (abrí una terminal nueva para usar "nudge pair").'
    }

    # 6. Firewall (paso opcional con administrador).
    $tcpRule = ((Invoke-NetshExitCode 'advfirewall firewall show rule name="Nudge"') -eq 0)
    $mdnsRule = ((Invoke-NetshExitCode 'advfirewall firewall show rule name="Nudge mDNS"') -eq 0)
    $hasRule = ($tcpRule -and $mdnsRule)
    $addRule = $false
    if ($FirewallRule -eq 'yes') {
        $addRule = $true
    } elseif ($FirewallRule -eq 'ask') {
        if ($hasRule) {
            Write-Host 'Las reglas del firewall de Nudge ya existen.'
        } else {
            Write-Host ''
            Write-Host 'Firewall: para que el teléfono encuentre y use esta PC, Windows tiene que'
            Write-Host 'permitir conexiones entrantes a Nudge en redes PRIVADAS: TCP 8765 (Nudge)'
            Write-Host 'y UDP 5353 (mDNS). Hace falta aceptar un aviso de administrador (UAC).'
            $addRule = Test-Yes (Read-Host '¿Agregar las reglas ahora? (s/N)')
        }
    }
    if ($addRule) {
        $fwScript = Join-Path $installDir 'firewall.ps1'
        $fwArgs = '-NoProfile -ExecutionPolicy Bypass -File "{0}" -Action add -Program "{1}"' -f $fwScript, $bgExe
        try {
            $proc = Start-Process -FilePath $psExe -ArgumentList $fwArgs -Verb RunAs -WindowStyle Hidden -Wait -PassThru
            if ($proc.ExitCode -eq 0) {
                Write-Host 'Reglas del firewall agregadas.' -ForegroundColor Green
            } else {
                Write-Host "No se pudieron agregar las reglas del firewall (código $($proc.ExitCode))." -ForegroundColor Yellow
            }
        } catch {
            Write-Host 'Reglas del firewall no agregadas (se canceló el permiso de administrador).' -ForegroundColor Yellow
            Write-Host 'Podés volver a ejecutar el instalador más tarde para agregarlas.'
        }
    } elseif (-not $hasRule) {
        Write-Host 'Sin reglas del firewall: si el teléfono no encuentra la PC, volvé a ejecutar el instalador.'
    }

    # 7. Iniciar el servidor.
    if (-not $NoStart) {
        $proc = Start-Process -FilePath $bgExe -WorkingDirectory $installDir -PassThru
        Start-Sleep -Seconds 2
        if ($proc.HasExited) {
            Write-Host "Nudge se cerró al iniciar. Mirá el log: $env:APPDATA\nudge\nudge.log" -ForegroundColor Yellow
        } else {
            Write-Host 'Nudge está corriendo en segundo plano.' -ForegroundColor Green
        }
    }

    Write-Host ''
    Write-Host 'Listo. Cuando un teléfono pida emparejarse, aparece una ventana Sí/No (por defecto No).'
    Write-Host "Log: $env:APPDATA\nudge\nudge.log"
}
catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    $exitCode = 1
}

Wait-Close
exit $exitCode
