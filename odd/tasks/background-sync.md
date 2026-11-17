# Feature: background-sync — sincronización de clipboard con la app cerrada

Mover el motor de sync de clipboard desde la pantalla remota a un
ForegroundService atado al último host elegido, activado automáticamente al
seleccionar host.

## Specs

- **S1** — Verbatim: "vamos a integrar ahora el background sync." Decisiones
  del usuario en este ciclo: (a) activación "Siempre tras elegir host" — no
  hay toggle: seleccionar un host arranca el servicio; (b) alcance "Último
  host elegido (recomendado)" — el servicio sincroniza solo con el host
  seleccionado y lo reintentora si desaparece, avisando en la notificación.

- **S2 — Servicio**: `ClipboardSyncService` (foreground service). Arrancar
  (`ensureStarted(context)`) desde: selección de host en MainActivity,
  onCreate de RemoteControlActivity y onCreate de MainActivity si existe
  `selected_host` persistido. `START_STICKY`. `targetSdk 34` obliga tipo de
  FGS: usar `dataSync` con sus permisos de manifiesto
  (`FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_DATA_SYNC`, `POST_NOTIFICATIONS`).
  Limitación documentada (no es scope): tras reinicio del teléfono el
  servicio no arranca solo — la primera apertura de la app lo reactiva.

- **S3 — Motor**: el protocolo actual (ClipItem, lastSyncedItem, listener +
  polling 2 s, push/pull de texto e imagen con opt-in `share_images`) se
  EXTRAEE a la lógica del servicio; RemoteControlActivity deja de correr su
  propio motor (un solo dueño del sync, sin doble loop). La pantalla remota
  muestra el estado leyendo del servicio (broadcast/local broadcast o estado
  estático simple) y conserva su checkbox `share_images` (prefs globales
  actuales, el servicio los relee).

- **S4 — Notificación**: canal único "Clipboard Share". Texto:
  `Clipboard sincronizado con <host>` cuando hay conexión; si el host no
  responde tras N reintentos (failures ≥ 3 consecutivos),
  `Sin conexión con <host> — reintentando`. Tap abre MainActivity. Sin sonido
  (low importance). Pedir POST_NOTIFICATIONS en API 33+ al primer arranque
  del servicio desde la UI.

- **S5 — Sin cambios de protocolo server** (endpoints actuales) ni del
  remote control por WebSocket, que sigue vivo solo con su pantalla abierta.

- **S6 — Prueba E2E obligatoria**: (a) elegir host → servicio corriendo
  (notif visible en `dumpsys notification`) → app a background → `wl-copy`
  en PC → dentro de ~3 s el celu recibe el texto (verificable por log del
  servicio en logcat); (b) copia manual en el celu por el usuario →
  `GET /clipboard/text` del server muestra el texto — **solo con la app
  visible** (ver L3: sin foco Android 10+ niega la lectura); (c) matar el server →
  la notif pasa a "Sin conexión" y al levantarlo vuelve sola (retry loop
  del servicio), sin reabrir la app.

## Tasks

- T1 — Implementación Android (servicio + extracción del motor + manifest +
  notif + permisos). Route: worker, superficie android/**.
- T2 — Verificación estructural + E2E (a) y (c) por adb; (b) con un toque
  del usuario. Route: parent.
- T3 — Docs: README "Estado del proyecto" + PLAN ítem 2 marcado como hecho
  con limitación de reboot; Log del doc. Route: parent.

## Log

- L1 — Contexto: tras host-picker + rename por hostname. El review nativo
  de los candidatos previos (host-picker, rename) quedó pendiente por el
  update de pi; se hará sobre la unión al cerrar este feature.
- L2 — T2 E2E por adb WI-FI (sin cable): (a) app a HOME + wl-copy PC →
  "remote applied" en logcat del servicio a los ~3 s; (c) kill server → 3x
  "Sync error"; restart → "remote applied: final-recovery" sin abrir la app;
  notif actualizada (PluginAOD onNotificationInfoUpdated com.clipboardshare).
  Singleton de ctl.sock verificó por accidente: segundo server rechazado.
  Falta (b): copia manual del usuario celu→PC.
- L3 — Review nativo (opus, review-reliability + refuter) halló R3-001
  CRITICAL: en Android 10+ el servicio no puede leer el clipboard sin foco,
  así que celu→PC jamás ocurre en background y README/spec lo afirmaban.
  El usuario ya lo había reportado ("solo sincroniza cuando entro a la
  app"); medido: 4 denegaciones/8 s con la app en background (API 36).
  Corrección acotada: contador de visibilidad real, el servicio no intenta
  leer el clipboard local sin foco, el estado dice "celu → PC solo con la
  app abierta" y README/PLAN/spec ya no prometen el sync bidireccional en
  background. El arreglo real queda en odd/tasks/bg-phone-to-pc.md.
- L4 - Resultado del review nativo (lineage review-28f45c0bab8e3371, revisor
  anthropic/claude-opus-5-5): lente reliability + refuter confirmaron R3-001;
  la correccion acotada (bfdd985, 80 lineas) fue RECHAZADA por el validator
  (estado escalated, terminal, sin aprobacion). Motivo: reescribir el alcance
  no cumple el criterio original; ademas en background un cambio nuevo del
  PC sigue pisando una copia del celu que nunca se envio. Seguimientos del
  validator: (1) el gate de visibilidad aplica tambien por debajo de
  Android 10 (minSdk 24), donde la lectura en background si esta permitida;
  (2) el spec bg-phone-to-pc debe estar versionado (lo esta desde este
  commit); (3) "activity started" no es "ventana con foco" (multiventana).
  Costo total ~$0.45. El arreglo real sigue en odd/tasks/bg-phone-to-pc.md.
- L5 - R3-001 resuelto de raiz por odd/tasks/bg-phone-to-pc.md (celu a PC real
  en background, verificado en el celu). El intento de corregirlo solo con
  visibilidad/timestamp (idea de comparar ClipDescription.timestamp) quedo
  refutado en el dispositivo: en Android 16 sin foco la descripcion del clip
  es null. Ese parche no se commiteo.
