# Linux app: Nudge name, icon and autostart
Rename the Linux server, panel and Arch package to Nudge with the new icon, and add user-controlled autostart. Branch: feat/input-control. Base: 6cc5cdf. Out of scope for now: the Windows installer (next feature).

## Specs
- S1: "aún está con el nombre antiguo y el logo viejo" — the Linux app (panel, tray, launcher entries, notifications, README, package) is called Nudge and uses the new icon: a white cursor arrow with three teal motion trails on a teal gradient square (Android sources: ic_launcher_background.xml colors #0A8C91 to #0B3C3F diagonal, ic_launcher_foreground.xml paths with a 0.88 scale around the center).
- S2: The user chose "Todo, incluidos nombres internos". Names after the change: package `nudge` (with `replaces`/`conflicts`/`provides` of `clipboard-share`); server and CLI binary `nudge`; panel binary `nudge-panel`; systemd user unit `nudge.service`; config dir `~/.config/nudge` (control socket `ctl.sock` and `state.json` inside); desktop IDs `nudge.desktop` and `nudge-panel.desktop`; icon name `nudge`; udev rules file `99-nudge-uinput.rules`; mDNS service type `_nudge._tcp`; environment overrides `NUDGE_*`, still honoring the old `CLIPBOARD_SHARE_*` names as a fallback.
- S3: Migration, no data loss: when `~/.config/nudge` does not exist and `~/.config/clipboard-share` does, the server, CLI and panel move the old directory to the new name once, so already paired devices keep working. Never overwrite an existing `~/.config/nudge`.
- S4: The user asked for "una opción en la app para que se inicie automaticamente, el usuario debe de poder desactivarlo si así lo desea" and answered "Que hayan las 2 opciones y que el usuario decida. obvio si se lanza el front, también debe de lanzarse el servidor." — the panel has two independent switches: (a) start the server when the user logs in (systemd user unit enable/disable), (b) open the panel when the user logs in (XDG autostart: a user override `~/.config/autostart/nudge-panel.desktop` with `Hidden=true` disables the packaged default). Whenever the panel is launched, it starts the server if it is not running, whatever (a) says.
- S5: The user answered "Activado, con interruptor para apagarlo": both switches are on after installing, and the user can turn each one off from the panel.
- S6: The Android app discovers the new mDNS type `_nudge._tcp.` (change the constant in MainActivity.kt); everything else about discovery is unchanged.
- S7: Existing behavior preserved: pairing, approve/deny/revoke, clipboard sync and remote input work as before; Go tests (`go test ./...`) and the panel tests (`panel/bin/test`) pass; the Android build and unit tests pass.
- S8: Docs updated: README.md and pkgbuild/README.md describe Nudge, the new names, the migration and the two autostart switches.

## Tasks
- T1 (S2, S3, S6, S7) route: worker W1 — Go server/CLI/module renames, `NUDGE_*` env with fallback, config-dir migration, mDNS type, Android constant. Risk: item 1/3 (stored data migration, CLI and config contracts). Own commit. Commit: pending.
- T2 (S1, S2, S4, S5, S7, S8) route: worker W2 — panel (names, `nudge-panel`, config dir/socket paths, icon SVG, autostart switches, server start on launch), pkgbuild (PKGBUILD, desktop files, unit, udev rules, icon install), READMEs. Commit: pending.
- T3 verification: build the package with makepkg (no install), run all test suites, run the migration against a temp HOME. Commit: pending.

## Log
- L1: Ahora si haz el commit. leugo vamos a preparar el instalador para windows. por cierto, te falta ajustar la app para arch que creamos. aún está con el nombre antiguo y el logo viejo, hay que poner una opción en la app para que se inicie automaticamente, el usuario debe de poder desactivarlo si así lo desea.
- L2: (answer) Renombrar: "Todo, incluidos nombres internos". Autoinicio: "Que hayan las 2 opciones y que el usuario decida. obvio si se lanza el front, también debe de lanzarse el servidor." Por defecto: "Activado, con interruptor para apagarlo (recomendado)".
