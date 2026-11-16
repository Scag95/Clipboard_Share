# Feature: bg-phone-to-pc — envío celu → PC automático con la app en background

Android 10+ niega la lectura del clipboard a apps sin foco. El servicio de
sync solo propaga PC → celu en background. Hacer celu → PC automático con la
técnica de KDE Connect: detectar cada copia por el log del sistema y leer el
clipboard desde una activity transparente fugaz.

## Specs

- **S1** — Verbatim: "Solo sincroniza el clipboard cuando entro a la app."
  Decisión del usuario: "Automático con READ_LOGS + superposición" (totalmente
  automático; acepta fragilidad entre versiones de Android y dar los
  permisos por adb).

- **S2 — Evidencia del mecanismo (ya medida)**: con la app en background el
  sistema registra `ClipboardService: Denying clipboard access to
  com.clipboardshare, application is not in focus` (4 en 8 s con polling de
  2 s; API 36). Ese mismo log lo emite el sistema cuando el clipboard cambia
  y despacha a un listener sin foco.

- **S3 — Detección**: `ClipboardSyncService` mantiene registrado el
  `OnPrimaryClipChangedListener` y arranca un lector de logcat
  (`logcat -v brief -b main -T 1 ClipboardService:E *:S`, proceso hijo,
  reiniciado si muere con backoff) que reacciona SOLO a líneas que contienen
  `Denying clipboard access to com.clipboardshare`. Cada línea (con debounce
  de ~400 ms) = "el clipboard cambió" → disparar S4.

- **S4 — Lectura fugaz**: `ClipboardReadActivity` (tema translúcido, sin UI,
  `excludeFromRecents`, `noHistory`, `taskAffinity` vacío). El servicio la
  lanza con `FLAG_ACTIVITY_NEW_TASK` (permitido desde background porque la
  app tiene SYSTEM_ALERT_WINDOW). En `onWindowFocusChanged(hasFocus=true)`
  lee `primaryClip`, entrega el item al motor del servicio para el push
  (mismo protocolo `lastSyncedItem`, mismo manejo de `share_images`), y hace
  `finish()` sin animación. Máximo 1 lanzamiento en vuelo; si el foco no
  llega en 1.5 s, abortar y loguear.

- **S5 — Sin falsos disparos**: el polling periódico NO lee el clipboard
  local mientras la app no está visible (esas lecturas denegadas generarían
  líneas de log y bucles de lanzamiento). Con la app visible el comportamiento
  actual no cambia (lectura directa). PC → celu sigue por polling del server
  en background, intacto.

- **S6 — Permisos y diagnóstico**: manifest `READ_LOGS` y
  `SYSTEM_ALERT_WINDOW`. Si falta alguno, `clipboardStatus` lo dice en la
  pantalla remota ("Falta permiso READ_LOGS: ver README") y el servicio sigue
  funcionando en modo reducido (PC → celu + foreground). No pedir los
  permisos por UI (READ_LOGS no es otorgable por el usuario normal); se dan
  por adb: `adb shell pm grant com.clipboardshare android.permission.READ_LOGS`
  y `adb shell appops set com.clipboardshare SYSTEM_ALERT_WINDOW allow`.

- **S7 — Sin cambios en el server ni en el protocolo HTTP.**

## Tasks

- T1 — Implementación Android (servicio + activity + manifest + estado).
  Route: worker, superficie android/**.
- T2 — Padre: dar permisos por adb, reinstalar, E2E real: copiar texto dentro
  de una app del celu con la app de Clipboard Share en background →
  `curl /clipboard/text` en la PC lo muestra; copiar imagen con opt-in off →
  no se sube; matar/relevantar logcat.
- T3 — Docs + commit.

## Log

- L1 — Hallazgo: tras bg-sync el usuario reporta "Solo sincroniza el
  clipboard cuando entro a la app". Reproducido: restricción de Android 10+
  (lectura sin foco denegada). Opciones presentadas: botón/tile, automático
  con READ_LOGS+overlay, dejarlo. Elegida: automático.
- L2 - El review nativo del bg-sync (opus) confirmo que reescribir el
  alcance no basta: el criterio original exige celu->PC real en background.
  Este spec es el arreglo. Al implementarlo: gatear la lectura sin foco solo
  en Build.VERSION.SDK_INT >= Q (minSdk 24 permite lectura en background en
  versiones viejas) y no aplicar un clipboard remoto sobre uno local que no
  se pudo leer/enviar.
