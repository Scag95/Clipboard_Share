# Nudge: agrega o quita las reglas de entrada del Firewall de Windows.
#
# Lo ejecutan install.ps1 y uninstall.ps1 con permisos de administrador
# (Start-Process -Verb RunAs); también se puede correr a mano desde una
# PowerShell de administrador:
#   firewall.ps1 -Action add -Program "C:\...\Nudge\nudge-background.exe"
#   firewall.ps1 -Action remove
#
# Reglas (solo redes privadas, solo para el programa indicado):
#   "Nudge"       TCP 8765 entrante  (API HTTP que usa el teléfono)
#   "Nudge mDNS"  UDP 5353 entrante  (descubrimiento automático mDNS)
#
# Códigos de salida: 0 bien, 2 sin permisos de administrador,
# 3 netsh falló, 4 parámetros inválidos. Compatible con Windows PowerShell 5.1.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('add', 'remove')]
    [string]$Action,
    [string]$Program = ''
)

$ErrorActionPreference = 'Stop'

# Ejecuta netsh.exe con la línea de argumentos exacta (sin re-citado de
# PowerShell) y devuelve el código de salida y la salida de texto.
function Invoke-Netsh([string]$Arguments) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = Join-Path $env:SystemRoot 'System32\netsh.exe'
    $psi.Arguments = $Arguments
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $out = $p.StandardOutput.ReadToEnd()
    $err = $p.StandardError.ReadToEnd()
    $p.WaitForExit()
    return New-Object PSObject -Property @{ ExitCode = $p.ExitCode; Output = ($out + $err) }
}

try {
    $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        Write-Host 'Hace falta ejecutar este script como administrador.' -ForegroundColor Red
        exit 2
    }

    $names = @('Nudge', 'Nudge mDNS')

    # Borrar primero las reglas con estos nombres: así "add" no duplica
    # reglas al reinstalar. Solo se borran reglas con estos nombres exactos.
    foreach ($n in $names) {
        [void](Invoke-Netsh ('advfirewall firewall delete rule name="{0}"' -f $n))
    }

    if ($Action -eq 'remove') {
        foreach ($n in $names) {
            $r = Invoke-Netsh ('advfirewall firewall show rule name="{0}"' -f $n)
            if ($r.ExitCode -eq 0) {
                Write-Host "No se pudo borrar la regla '$n'." -ForegroundColor Red
                exit 3
            }
        }
        Write-Host 'Reglas del firewall de Nudge borradas.'
        exit 0
    }

    if ([string]::IsNullOrEmpty($Program) -or $Program.Contains('"')) {
        Write-Host 'Falta -Program con la ruta de nudge-background.exe.' -ForegroundColor Red
        exit 4
    }

    $rules = @(
        ('advfirewall firewall add rule name="Nudge" dir=in action=allow protocol=TCP localport=8765 profile=private program="{0}" enable=yes' -f $Program),
        ('advfirewall firewall add rule name="Nudge mDNS" dir=in action=allow protocol=UDP localport=5353 profile=private program="{0}" enable=yes' -f $Program)
    )
    foreach ($args0 in $rules) {
        $r = Invoke-Netsh $args0
        if ($r.ExitCode -ne 0) {
            Write-Host "netsh falló ($($r.ExitCode)): $($r.Output)" -ForegroundColor Red
            exit 3
        }
    }
    Write-Host 'Reglas del firewall de Nudge agregadas.'
    exit 0
}
catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 3
}
