# Feature: transparent-pairing — pairing sin PIN, con aprobación en el PC

Reemplazar el flujo "tipear PIN en el celu en cada arranque del server" por
identidad de dispositivo + estado persistido: el primer emparejamiento se
aprueba en el PC (notificación + CLI), y después el celu se reconecta solo,
sobreviviendo reinicios de ambos lados.

## Specs

- **S1** — Verbatim: "quiero mejorar la conectividad entre 2 dispositivos,
  eso de estar meitendo el pin y conectandose, no es cómodo, que opciones
  tenemos para que sea transparente para el usuario?" Objetivo: después del
  primer emparejamiento, el usuario nunca más tipea un código; reconexión
  automática tras reboot del server o del celu.
- **S2** — Decisión del usuario (opción elegida): "A + aprobación en el PC".
  Mecanismo: identidad de dispositivo persistente + tokens persistidos en
  servidor + aprobación explícita de dispositivos NUEVOS en el PC. El PIN
  de 6 dígitos se elimina por completo junto con su lockout por intentos.
- **S3** — Protocolo server (JSON; reemplaza POST /pair):
  - `POST /pair/request` body `{"id":"<uuid-device>","name":"<modelo>"}`.
    - Dispositivo ya aprobado conocido → `200 {"token":"<nuevo>"}` inmediato
      (re-pair transparente; el token viejo se revoca al emitir el nuevo).
    - Desconocido → `202 {"requestId":"<opaque>"}`; caduca a 90 s; se lanza
      `notify-send "Clipboard Share" "Pair request from <name> — clipboard-share pair --last"`
      (best-effort; sin display, se ignora y se aprueba por CLI).
    - Más de 5 requests pendientes sin resolver desde la misma IP → `429`.
  - `GET /pair/status?id=<requestId>` desde la MISMA IP que lo creó →
    `{"state":"pending|expired|denied|approved"}`; cuando una persona aprueba
    en el PC, la siguiente status devuelve `200 {"state":"approved",
    "token":"..."}` una única vez.
  - Estado en `~/.config/clipboard-share/state.json`, modo `0600`:
    lista de dispositivos `{id, name, token, ip, firstApproved}` + pendientes
    en memoria. Tokens válidos sobreviven el restart del server.
  - Recovery manual documentado: `clipboard-share pair revoke --device <id>`
    y, si se pierde el celu, borrar `state.json` resetea todo.
- **S4** — CLI de control sobre unix socket
  `~/.config/clipboard-share/ctl.sock` (solo usuario del server):
  - `clipboard-share pair list` — pendientes y aprobados.
  - `clipboard-share pair --last` — aprueba el request pendiente más nuevo
    (pensado para un keybinding de Hyprland; documentar ejemplo).
  - `clipboard-share pair deny --request <id>`, `clipboard-share pair revoke
    --device <id>`.
  - Sin server corriendo: error claro, exit 1.
- **S5** — Android (RemoteControlActivity): sin campo de PIN.
  - `UUID` generado y guardado en prefs al primer arranque + `Build.MODEL`
    como nombre.
  - "Conectar" → POST /pair/request → si 202, polls GET /pair/status cada
    1.5 s con botón "Cancelar"; si 200, guarda token y abre el WebSocket.
  - WS 401 o token desconocido → re-request automático (200 inmediato para
    dispositivo conocido → reconexión transparente tras reboot del server).
  - El celu mantiene UNA sola identidad/tokens en prefs; UI muestra nombre
    del dispositivo aprobado.
- **S6** — Los endpoints de clipboard siguen sin autenticación (igual que
  antes; sin cambios de comportamiento). El token solo autoriza /ws/input,
  con vínculo IP como hoy.
- **S7** — No hay migración del PIN viejo (beta personal): tras el cambio,
  el celu debe aprobarse una vez nueva en el PC. README y PLAN actualizados
  en el mismo feature.

## Tasks

- T1 — Server: paquete `internal/pair` nuevo modelo (request/status/aprobación
  + estado persistido + socket) con tests. Route: worker. Dep S3,S4.
- T2 — Server: subcomandos CLI `pair` y notificación; wiring en
  `cmd/server`. Route: worker (mismo que T1). Dep S4.
- T3 — Android: RemoteControlActivity sin PIN + identidad + flujo
  request/status/polling + persistencia token. Route: worker. Dep S5.
- T4 — Docs: README (flujo nuevo, keybinding, recovery), PLAN (ítems
  seguridad que esto resuelve/cierra). Route: parent. Dep S7.
- T5 — E2E: server nuevo + celu real; primera aprobación por CLI; reboot
  server → reconexión sola. Route: parent + usuario.

## Log

- L1 — Petición original verbatim: "Ok, funciona. ahora. quiero mejorar la
  conectividad entre 2 dispositivos, eso de estar meitendo el pin y
  conectandose, no es cómodo, que opciones tenemos para que sea transparente
  para el usuario?"
- L2 — Decisión de opción: el usuario eligió "A + aprobación en el PC"
  (persistencia de estado + trust-on-first-pair aprobado en el PC). El PIN
  se elimina.
- L3 — Workers T1-T3 completados. Cross-check del padre: el server responde
  410 en expired y Android lo traducía a Failed (nunca llegaba al UI de
  Expired); corregido mapeando 410→Expired en InputClient.status(). Nota:
  S3/S6 mencionaban vínculo token↔IP heredado del WIP descartado de la
  sesión paralela; el código real valida solo token, y ambos workers lo
  conservaron correctamente (Validate(token)).
