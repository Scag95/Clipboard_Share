# Feature: linux-beta — servidor Linux + APK debug para beta personal

Preparar la primera beta de Clipboard Share para uso personal: un APK debug
(sin publicación), servidor que corra en Linux además de Windows para poder
hacer pruebas reales en esta máquina, y sync solo con la app abierta
(background sync queda para después).

## Specs

- **S1** — Verbatim: "la app es para uso personal, no la voy a publicar con
  generar un apk es suficiente." Entregable: `app-debug.apk` instalable por
  USB/cable; sin Play Store, sin firma comercial, sin ProGuard ceremony.
- **S2** — Verbatim: "En cuando a windows. hagamoslo compatible también para
  linux, así podemos hacer pruebas." Entregable: `clipboard-share` binario
  Linux que corre el servidor completo (clipboard + mDNS + pairing +
  `/ws/input`) en Wayland/Hyprland local; la inyección de input en Linux usa
  uinput (Wayland no permite inyección por protocolo para puntero).
- **S3** — Verbatim: "por ahora podemos dejar la app que funcione solo cuando
  este abierta. luego implementamos el background sync." Scope beta: sync de
  clipboard y remote control activos únicamente con la actividad visible. No
  se agrega ForegroundService en esta etapa.
- **S4** — Verbatim: "si necesitas el sdk de android studio avisame y lo
  instalamos." Autorización explícita de instalación del toolchain Android
  (OpenJDK 17, Gradle, Android SDK platform-34 + build-tools) en paths de
  usuario (`~/.local/share/mise`, `~/Android/Sdk`).

## Tasks

- T1 — Probe clipboard del server en Linux/XWayland (run real + `wl-paste`).
  Ruta: parent inline. Commit: con T2 o separable.
- T2 — `internal/input`: injector Linux vía uinput (move/click/scroll/key/text
  ASCII) + setup de permisos `/dev/uinput` documentado. Depende de T1 para
  contexto de plataforma. Route: worker (referencia este doc, S2).
- T3 — Toolchain Android: java@17 + cmdline-tools + SDK 34 + gradle (job en
  background), y wrapper `gradlew` commiteado en `android/`. Route: parent.
- T4 — Compilar `android/` real, arreglar errores de Kotlin/layouts y generar
  `app-debug.apk` (`assembleDebug`). Depende de T3. Route: worker tras T3.
- T5 — E2E: server local en Linux + celu del usuario: pairing PIN, sync texto
  (S1/S3), touchpad (S2), lockout 423. Route: parent + usuario con dispositivo.
- T6 — Docs beta: README "probar en Linux", setup uinput, instalar APK debug,
  firewall XWayland/no necesaria; cerrar PLAN ítems cubiertos. Route: parent.

## Log

- L1 — Petición original verbatim: "Vale, que hace falta para que la
  aplicación tenga la primera beta?" seguido de la decisión: "la app es para
  uso personal, no la voy a publicar con generar un apk es suficiente. En
  cuando a windows. hagamoslo compatible también para linux, así podemos hacer
  pruebas. por ahora podemos dejar la app que funcione solo cuando este
  abierta. luego implementamos el background sync. si necesitas el sdk de
  android studio avisame y lo instalamos."
- L2 — Hallazgo T1: en Hyprland/Wayland la librería X11 lee el clipboard
  pero las escrituras no llegan (wl-paste "Nothing is copied"); resuelto con
  backend wl-clipboard (commit 4931d99). Dos gaps del worker corregidos por
  el padre: hang del daemon wl-copy sobre el pipe de stderr (WaitDelay +
  exit-code 0) y nil-vs-empty args en el helper de tests.
- L3 — E2E server-linux verificado desde worktree limpio del HEAD 354b3c5:
  health, texto GET/POST vía wl-copy/wl-paste reales, imagen PNG GET/POST
  (200/204, tipo image/png), /pair PIN 200 y PIN malo 401. Faltan: udev
  uinput (necesita sudo del usuario) y prueba con dispositivo físico.
- L4 — Independent verifier (mwf3dnoj-4-xq14) sobre 4931d99..bc4a1a5:
  reproducidos y corregidos (1) overflow int32 en Move/Scroll fuera de rango
  (MaxInt llegaba a -1) → clampDelta antes de emitir; (2) mods desconocidos
  se ignoraban en silencio → ahora error antes de emitir ningún evento;
  (3) UI_DEV_DESTROY no corría si setup fallaba tras crear el dispositivo →
  NewLinux cierra con destroy. Probes fijados como tests
  (linux_sender_bounds_test.go, linux_sender_modifier_test.go).
  Rechazado el probe "No suitable type of content copied = error": para
  GET /clipboard/image ese caso significa "el clipboard no tiene imagen" →
  nil/204 es la semántica correcta del producto (y ya estaba fijada por
  test del worker).
- L5 — Verifier independiente sobre e6b7b41: APROBADO, sin blockers.
  RED→GREEN reproducido en el padre vs commit (Move(MaxInt) emitía (-1,0),
  ahora clamp ±1000; Key con mod inválido devolvía nil, ahora error
  atómico antes de emitir). Destroy post-setup-failure verificado por
  lectura (sin privilegios no es inyectable). La sonda durable sugerida se
  omitió: ya cubierta por linux_sender_bounds_test.go y
  linux_sender_modifier_test.go.
