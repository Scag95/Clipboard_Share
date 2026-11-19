# Windows polish: exe icon and version info, dialog tone, tray icon
Three improvements found while looking at the Windows build under Wine. Branch: feat/windows-polish. Base: feat/windows-build at 1096f11.

## Specs
- S1: The user asked: "haz las 3 mejeroas que propusiste." The three proposals were: "Ícono y datos de versión embebidos en los `.exe`: cambio chico en `scripts/build-windows.sh` con `go-winres`."; "Mismo tono en el diálogo que en el resto: vos o tú, como prefieras."; "Ícono en la bandeja de Windows: es la mejora con más impacto visible, pero es una funcionalidad nueva y más grande."
- S2 (icon and version): `nudge.exe` and `nudge-background.exe` built by `scripts/build-windows.sh` embed the Nudge icon (from `pkgbuild/nudge.svg`, same sizes as `nudge.ico`) as icon group resource ID 1, and a version resource (ProductName "Nudge", FileDescription, CompanyName/LegalCopyright per LICENSE, FileVersion/ProductVersion = the script's version) so Explorer, the taskbar and Properties show them. Uses `go-winres` run as a pinned build tool (`go run github.com/tc-hib/go-winres@<pinned version>`), no change to go.mod runtime dependencies. The script must still write nothing in the repository besides `dist/` (generated .syso files are removed even on failure). The CI `package` job keeps working.
- S3 (tone): the pairing dialog uses voseo like the installer and README: "Si no reconoces este dispositivo, pulsa No." becomes "Si no reconocés este dispositivo, elegí No." (and any other tú forms in Windows-visible strings). Tests updated.
- S4 (tray, user decisions): the Windows server (both `nudge.exe` server mode and `nudge-background.exe`; never the `nudge pair` CLI) shows a notification-area icon built with Win32 by hand (Shell_NotifyIconW, a hidden window, TrackPopupMenu) using golang.org/x/sys already in go.mod: no new dependencies. Icon: the exe's icon resource ID 1 when present, otherwise the stock application icon. Tooltip "Nudge". Right click opens a menu with only: the server status (for example "Escuchando en el puerto 8765") and the paired phones (name and IP, informational, disabled items; "Ningún teléfono emparejado" when empty). User choice: "Estado y teléfonos emparejados" only (no revoke, no autostart switch, no log, no quit item).
- S5 (pairing notice, user decision "Notificación + diálogo"): when a new phone requests pairing, the tray shows a Windows notification (balloon/toast via NIF_INFO) "Solicitud de emparejamiento" with the phone name, and the existing Yes/No dialog still appears as today and stays the only place to approve. Text coming from the LAN is sanitized and truncated exactly like the dialog.
- S6: If the tray cannot be created (no shell, Explorer restarting), the server keeps running and logs it; the icon is re-added when Explorer restarts (TaskbarCreated message). Linux behavior unchanged. `go build ./... && go vet ./... && go test ./...` pass on Linux, `GOOS=windows go vet ./...` passes, and the platform-independent parts (menu model, notification text) have tests runnable on Linux.

## Tasks
- [ ] T1 (S2) route: worker W-icon — scripts/build-windows.sh, packaging/windows/winres/**, .github/workflows/windows.yml (only if the package job needs it). Commit: pending.
- [ ] T2 (S3) route: inline — cmd/server/approval.go and its test. Commit: pending.
- [ ] T3 (S4-S6) route: worker W-tray — cmd/server/** except approval text. Commit: pending.
- [ ] T4 docs + verification: docs/windows.md, README.md, packaging/windows/README-windows.txt; Wine run of the tray and the zip; CI run. Commit: pending.

## Log
- L1: haz las 3 mejeroas que propusiste.
- L2 (decisions): menu "Estado y teléfonos emparejados"; requests "Notificación + diálogo (recomendado)"; implementation "Win32 a mano (recomendado)"; base "Subir feat/windows-build y rama nueva encima" (pushed 1096f11). Tone: voseo, matching the installer ("Presioná Enter") and the README.
