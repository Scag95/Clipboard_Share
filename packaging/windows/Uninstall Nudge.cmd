@echo off
rem Nudge: double-click launcher for uninstall.ps1.
rem Keep this file pure ASCII: cmd.exe reads it with the OEM code page.
rem Leave this folder first so it is never the current directory of a process.
rem Save the script folder before cd: when this file is started through a
rem quoted relative path, cmd recomputes %~dp0 from the new current directory.
setlocal
set "NUDGE_SRC=%~dp0"
if not exist "%NUDGE_SRC%uninstall.ps1" goto missing
cd /d "%USERPROFILE%"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%NUDGE_SRC%uninstall.ps1" %*
exit /b %ERRORLEVEL%
:missing
echo Falta uninstall.ps1 junto a este archivo.
echo Hay que extraer el zip completo (clic derecho, Extraer todo).
pause
exit /b 1
