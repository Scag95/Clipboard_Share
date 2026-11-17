# Feature: host-picker — home de hosts + trackpad directo

Rediseño de la app Android: la pantalla de inicio pasa a ser una lista de
hosts de la casa; elegir uno lleva directo al trackpad. Sincronización de
clipboard ligada al host elegido, con su estado y toggle dentro de la
pantalla remota.

## Specs

- **S1** — Verbatim: "quiero cambiar la pantalla de inicio. Primero quiero
  que me salgan los host en casa, puedo tener varios ordenadores a los que
  quiera estar conectado. una vez seleccione el host, ya me pasa directamente
  a la pantalla con el trackpad, debajo del trackpad, 2 botones en el mismo
  estilo que el trackpack simulando click derecho y click izquierdo. quiero
  seguir pudiendo dar click con un solo toque." Decisiones del usuario en
  este ciclo: (a) "Pestaña: sync sigue al host" — elegir host conecta su
  clipboard y el checkbox de imágenes + estado viven en un sector de la
  pantalla del trackpad; (b) la lista muestra hosts vivos **y** recordados
  marcados "sin respuesta", tocarlos reintenta su última IP.

- **S2 — Home (MainActivity)**: solo lista de hosts. Fuente: descubrimiento
  NSD vivo (`_clipboard-share._tcp`) + registro persistido de hosts ya
  conocidos `{name, host, port, lastConnected}` en SharedPreferences. Cada
  fila: nombre del servicio (o modelo) + estado `en red` / `sin respuesta`.
  Fila viva → seleccionar host y abrir RemoteControl directamente. Fila
  recordada sin respuesta → try `/health` con timeout 1.5 s en IO; si
  responde, tratar como viva; si no, toast claro y permanecer en home.
  Sin botones de clipboard en home.

- **S3 — Remote (RemoteControlActivity)**: recibe el host seleccionado
  (extra del intent; nunca NSD global). Al abrir: si hay token para ESE host,
  conectar WebSocket directo; si no, flujo actual request→pending→approved
  (reutilizando transparent-pairing S3) con polling y Cancelar. Sin botón
  "Conectar" en el camino feliz. Sector superior compacto "Clipboard":
  estado de sync del host + checkbox "Sincronizar imágenes" (mismo prefijo
  de prefs que hoy, global, no por host).

- **S4 — Trackpad**: TouchpadView queda como está; DEBAJO, exactamente dos
  botones "CLICK IZQ" y "CLICK DER" con el mismo estilo visual del trackpad
  (mismo fondo redondeado/borde que TouchpadView, tamaño generoso). Los
  atajos existentes (CTRL+C/V/X/Z, Enter, Esc, texto) se conservan debajo de
  esos dos botones. Tap de un dedo = click izquierdo, long-press/dos dedos =
  derecho: comportamiento intacto (S1 última frase).

- **S5 — Tokens por host**: SessionStore pasa de token único a mapa
  host:port→token persistido en SharedPreferences. Reconexión transparente
  (reboot del server) solo para el host elegido. `revoke`/401→re-request
  sigue la semántica actual, por host.

- **S6 — Sync de clipboard**: el motor (listener + polling 2 s, protocol
  lastSyncedItem) se ejecuta en RemoteControlActivity ligado al host elegido;
  con la pantalla cerrada no hay sync (misma limitación de beta que hoy,
  solo cambia DÓNDE vive). MainActivity ya no sincroniza.

- **S7 — Sin cambios de protocolo server** ni endpoints. Windows/Linux del
  server unaffected. Idioma UI: español neutro como el resto.

## Tasks

- T1 — Implementación Android completa (MainActivity, RemoteControlActivity,
  SessionStore, layouts, estilos). Route: worker (android/**).
- T2 — Cross-check + assembleDebug + E2E adb (fila viva real, fila recordada
  inventada "sin respuesta", tap único, botones de click, reconexión).
  Route: parent + usuario.

## Log

- L1 — Petición original verbatim en S1. Respuestas de diseño: pestaña
  "sync sigue al host" y lista "vivos + recordados".
- L2 — HP-T2 E2E por adb (usuario solo desbloqueó): fila viva → entra
  directo al trackpad; reconexión TRANSPARENTE tras matar/levantar el
  server (token por host en prefs: token_192.168.0.14:8765); CLICK
  IZQ/DER + tap único + swipe aceptados sin errores app/server; fila
  recordada falsa "fantasma" mostró "sin respuesta" y el toque NO navegó
  (probe /health falló como spec S2). Prefs de test limpiadas.
