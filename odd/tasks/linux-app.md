# Feature: linux-app — panel gráfico + empaquetado para Linux

App de escritorio para Omarchy/Linux que reemplaza el CLI
`clipboard-share pair` por un panel (ventana + bandeja) y deja el servidor
instalable como servicio de usuario con autostart, ícono y entrada en el
launcher.

## Specs

- **S1** — Verbatim: "Arregla R1 y continua con la app." La "app para Linux"
  es la decisión previa del usuario: **panel gráfico + empaquetado**; el
  panel reemplaza `clipboard-share pair`, el empaquetado agrega servicio de
  usuario con autostart, ícono y entrada de launcher.

- **S2 — Panel (Qt Quick, C++)**: sigue la skill `omarchy-app`: `QGuiApplication`,
  estilo Material oscuro, acento y redondeo del tema de Omarchy en vivo,
  textos con `qsTr`, binario único compilado con `qmake6` + `make`.
  Nombre de la app: `clipboard-share-panel` (binario); id de escritorio e
  ícono: `clipboard-share`.

- **S3 — Habla con el servidor por el socket de control existente** (sin
  tocar el protocolo de Go): socket Unix en `CLIPBOARD_SHARE_CTL_SOCK` o
  `~/.config/clipboard-share/ctl.sock`; un comando JSON por línea
  (`{"cmd":"list"}`, `approve`, `approve-last`, `deny`+`request`,
  `revoke`+`device`), una respuesta JSON por línea
  `{ok,error,message,requests[],devices[]}`. No hay push: el panel refresca
  con `list` periódico (~1 s). Las solicitudes pendientes caducan a los 90 s.

- **S4 — Qué muestra y permite**: estado del servidor (socket de control alcanzable: ese socket lo sirve
  el propio proceso del servidor, así que no hace falta `/health`; nombre de
  host);
  solicitudes pendientes con **Aprobar** / **Rechazar** (nombre, IP, hace
  cuánto); dispositivos aprobados con **Revocar** (nombre, IP, primera
  aprobación). Si el servidor no responde, la ventana lo dice con la causa
  (no se oculta ni se degrada en silencio).

- **S5 — Bandeja**: ícono en la bandeja; clic alterna la ventana; cerrar la
  ventana la oculta (el panel sigue vivo); menú con Abrir y Salir. Una
  solicitud nueva se avisa desde la bandeja (clic abre el panel) y **nunca**
  abre la ventana ni toma foco (review R1-001: cualquiera en la LAN puede
  mandar solicitudes); `--background`
  arranca oculto (para autostart). Una sola instancia.

- **S6 — Teclado**: `?` muestra los atajos, `Esc` oculta, `Q` sale. No hay atajo
  para aprobar: aprobar es un clic deliberado (review R1-001).

- **S7 — Se puede probar sin pantalla**: el backend (cliente del socket,
  parseo de respuestas, estado) es una clase `QObject` con tests Qt Test que
  corren `offscreen` contra un servidor falso en un socket temporal.

- **S8 — Empaquetado Arch** (`pkgbuild/`): un `PKGBUILD` que compila el
  servidor Go y el panel y instala: `/usr/bin/clipboard-share`,
  `/usr/bin/clipboard-share-panel`, el servicio de usuario systemd
  `clipboard-share.service` (arranca con la sesión gráfica, reinicia si
  cae), la regla udev para `/dev/uinput` (`uaccess`, sin grupo `input`),
  `clipboard-share.desktop` (launcher), un autostart del panel con
  `--background`, y el ícono SVG. Dependencias: `wl-clipboard`, Qt6.

- **S9 — Documentación**: el README reemplaza las instrucciones del CLI de
  emparejamiento por el panel (el CLI `pair` se mantiene para scripts),
  describe instalación, servicio, atajo de Hyprland y atajos del panel.

## Tasks

- [x] **T1** (S1-S9) — este documento y el corte de slice. Route: inline. Commit: ver git log (docs(odd): linux-app feature doc)
- [x] **T2** (S3, S4, S7) — backend C++ + tests Qt Test (9 casos; RED observado antes de implementar). Route: inline, test-first. Commit: ver git log (feat(panel): control socket backend)
- [x] **T3** (S2, S4, S5, S6) — UI QML, bandeja, tema Omarchy, `qsTr`. Route: inline. Commit: `5b5535e` (UI 8/8 + backend 9/9 offscreen; en vivo: arranque oculto, aparece ante solicitud real, StatusNotifierItem registrado)
- [x] **T4** (S8) — PKGBUILD, servicio de usuario, udev, `.desktop`, ícono. Route: worker (archivos) + build inline. Commit: `1b8f29c` (makepkg OK; el paquete contiene los 7 paths; **instalar y arrancar el servicio requiere sudo del usuario: sin probar**)
- [ ] **T5** (S9) — README, E2E en esta máquina (servicio + panel + emparejar el celu), cierre. Route: inline. Commit: ver git log (README hecho; E2E con el paquete instalado pendiente del sudo del usuario)

## Riesgos

- Risk (T4): item 5 (entrega/entorno: instalador, servicio de usuario, regla
  udev) → T4 va en su propio commit.
- Risk (T2/T3): item none para el servidor Go: no se toca; el panel solo
  consume el protocolo de control ya existente.

## Corte de slice

La rama `feat/input-control` ya está muy por encima de ~400 líneas (≈7.7k en
34 commits; ya publicada hasta `a110d9d` salvo este trabajo). La app de Linux
vive en `panel/` y `pkgbuild/`, directorios nuevos y disjuntos del resto:
**cada tarea T2-T5 es su propio commit** y la app puede salir en un PR
aparte si el usuario prefiere partir la rama.

## Log

- **L1** — "Arregla R1 y continua con la app." (respuesta a la propuesta: app
  de Linux = panel gráfico + empaquetado, y arreglar el hallazgo R1-001 del
  review: no loguear el contenido del portapapeles; hecho en `a110d9d`).
- **L2** — Mapa del protocolo de control obtenido por exploración de solo
  lectura (`internal/pair/pair.go:495-628`, `cmd/server/main.go:141-202`):
  no hay push, `DeviceInfo` no tiene `lastSeen`; no existen PKGBUILD, unit,
  `.desktop` ni ícono en el repo.
- **L3** — Decisión de T2: se descarta `GET /health` del panel (spec S4 reescrita):
  el socket de control lo sirve el mismo proceso, alcanzarlo ya prueba que el
  servidor corre. Hallazgo de los tests: el `list` de refresco posterior a una
  acción borraba su error en <1 s; el error de acción ahora persiste hasta la
  próxima acción. Sin catálogos i18n por ahora: textos con `qsTr`/`tr`, en
  inglés, sin el `Translator` de la plantilla (S2 sigue cumplida).
- **L4** — Hallazgos de la verificación en vivo de T3: (a) `tray.available` no es
  estático en QML (warning `undefined`): se usa la instancia; (b) en una
  ventana tileada las tarjetas se estiraban a todo el ancho: contenido
  centrado con tope de 560 px; (c) el diálogo de revocar tenía un binding
  loop de `implicitWidth`: ancho explícito. Probar teclas con `wtype` sobre la
  sesión viva del usuario no es fiable ni inocuo: la interacción se verifica
  con tests de UI offscreen (`panel/tests/ui_tests.cpp`).
- **L5** — El worker de T4 dejó `makepkg` bloqueado por `go` (solo provisto por
  mise); se construyó con `mise exec go@1.24 -- makepkg -f --nodeps --nocheck`
  (solo salta el chequeo de la dependencia de build).
- **L6** — El review nativo (opus, 4 lentes + refuter) encontró R1-001 CRITICAL:
  una solicitud de la LAN abría la ventana con foco y la tecla `A` sin
  modificador aprobaba la más nueva, así que teclear en otra app en el momento
  justo emparejaba a un atacante con control de mouse y teclado. Reproducido
  con dos tests en RED antes de corregir. Corrección: la solicitud se avisa
  desde la bandeja y no abre la ventana; se elimina el atajo `A`.
