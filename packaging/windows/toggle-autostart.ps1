# Nudge: activa o desactiva el inicio automático al iniciar sesión.
#
# Uso:  toggle-autostart.ps1 [on|off|status|toggle] [-NoPause]
#   on      crea el acceso directo "Nudge.lnk" en la carpeta Inicio del usuario
#   off     borra ese acceso directo (solo ese archivo)
#   status  muestra el estado actual
#   toggle  (por defecto) cambia el estado
#
# Solo toca el acceso directo del autoarranque: no inicia ni detiene el
# servidor. Compatible con Windows PowerShell 5.1.
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('on', 'off', 'status', 'toggle')]
    [string]$State = 'toggle',
    [switch]$NoPause
)

$ErrorActionPreference = 'Stop'

function Wait-Close {
    if ($NoPause) { return }
    try { [void](Read-Host 'Presioná Enter para cerrar') } catch { }
}

$exitCode = 0
try {
    if ([string]::IsNullOrEmpty($env:LOCALAPPDATA)) {
        throw 'No se encontró la variable LOCALAPPDATA.'
    }
    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\Nudge'
    $exe = Join-Path $installDir 'nudge-background.exe'
    $icon = Join-Path $installDir 'nudge.ico'
    $startupDir = [Environment]::GetFolderPath('Startup')
    if ([string]::IsNullOrEmpty($startupDir)) {
        throw 'No se encontró la carpeta Inicio del usuario.'
    }
    $link = Join-Path $startupDir 'Nudge.lnk'

    $isOn = Test-Path -LiteralPath $link
    $target = $State
    if ($State -eq 'toggle') {
        if ($isOn) { $target = 'off' } else { $target = 'on' }
    }

    switch ($target) {
        'on' {
            if (-not (Test-Path -LiteralPath $exe)) {
                throw "Nudge no está instalado ($exe no existe). Ejecutá primero 'Install Nudge.cmd'."
            }
            if (-not (Test-Path -LiteralPath $startupDir)) {
                [void](New-Item -ItemType Directory -Path $startupDir -Force)
            }
            # Recrear siempre: deja el acceso directo actualizado (idempotente).
            $shell = New-Object -ComObject WScript.Shell
            $lnk = $shell.CreateShortcut($link)
            $lnk.TargetPath = $exe
            $lnk.Arguments = ''
            $lnk.WorkingDirectory = $installDir
            if (Test-Path -LiteralPath $icon) { $lnk.IconLocation = "$icon,0" }
            $lnk.Description = 'Nudge (servidor en segundo plano)'
            $lnk.Save()
            Write-Host 'Inicio automático: ACTIVADO (Nudge arranca al iniciar sesión).' -ForegroundColor Green
        }
        'off' {
            if (Test-Path -LiteralPath $link) {
                Remove-Item -LiteralPath $link -Force
            }
            Write-Host 'Inicio automático: DESACTIVADO.' -ForegroundColor Yellow
        }
        'status' {
            if ($isOn) {
                Write-Host 'Inicio automático: ACTIVADO.'
            } else {
                Write-Host 'Inicio automático: DESACTIVADO.'
            }
        }
    }
}
catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    $exitCode = 1
}

Wait-Close
exit $exitCode
