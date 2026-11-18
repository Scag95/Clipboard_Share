@echo off
rem Nudge: double-click launcher for install.ps1 (per-user install, no admin).
rem Keep this file pure ASCII: cmd.exe reads it with the OEM code page.
if not exist "%~dp0install.ps1" goto missing
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
if "%ERRORLEVEL%"=="9009" goto nopowershell
exit /b %ERRORLEVEL%

:missing
echo Falta install.ps1 junto a este archivo.
echo Hay que extraer el zip completo (clic derecho, Extraer todo) antes de instalar.
pause
exit /b 1

:nopowershell
echo No se encontro powershell.exe (Windows PowerShell 5.1).
pause
exit /b 1
