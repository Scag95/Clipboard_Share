@echo off
rem Nudge: double-click launcher for uninstall.ps1.
rem Keep this file pure ASCII: cmd.exe reads it with the OEM code page.
rem Leave this folder first so it is never the current directory of a process.
if not exist "%~dp0uninstall.ps1" goto missing
cd /d "%USERPROFILE%"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall.ps1" %* & exit /b
:missing
echo Falta uninstall.ps1 junto a este archivo.
echo Hay que extraer el zip completo (clic derecho, Extraer todo).
pause
exit /b 1
