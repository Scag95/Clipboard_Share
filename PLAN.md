# Plan de implementación — Clipboard Share

Documento para retomar el trabajo en otro equipo/entorno.

## Estado actual (main)

- Servidor Go: clipboard real de Windows (texto + imagen), mDNS announce
  (`_clipboard-share._tcp`), HTTP API en `:8765`, apagado ordenado, límite de
  imagen de 10 MiB (413) y `ErrNoData` mapeado a respuestas vacías (204 / "").
- Cliente Android: descubrimiento NSD + sync bidireccional de texto e imagen
  (PNG) con `OnPrimaryClipChangedListener` y polling cada 2 s.
- Tests Go: handlers de `internal/api` con store inyectable.
- Repo: https://github.com/Scag95/Clipboard_Share (rama `main`).

## Pendientes / siguientes pasos

### 1. Modo manual de IP (debug/emulador)
- Agregar campo de texto en la UI de Android para ingresar la IP del servidor
  manualmente (útil para emulador, donde mDNS no funciona).
- Guardar la IP en SharedPreferences y usarla si el descubrimiento falla.

### 2. Sincronización en background
- Usar un `ForegroundService` en Android para mantener la sync activa
  mientras la app está en segundo plano.
- Polling periódico (WorkManager o coroutine con delay) para detectar cambios
  en el clipboard sin interacción del usuario.

### 3. Tests
- Go: tests unitarios para los handlers HTTP (`internal/api`). ✅ Hecho.
- Go: tests de `internal/clipboard` y `internal/discovery` (requieren seam de
  inyección o entorno real).
- Android: tests de descubrimiento y sync (Robolectric o instrumentados).
  Pendiente: el proyecto aún no tiene ni `androidTest/` ni `test/`.

### 4. Empaquetado
- Generar APK release firmado (`./gradlew assembleRelease`).
- Configurar firma (keystore) y ProGuard/R8.
- Documentar en README los pasos de build release.

### 5. Seguridad / robustez
- Autenticación simple (token compartido) entre PC y móvil. ✅ Hecho en rama
  `feat/input-control` (pairing por PIN de 6 dígitos + token de sesión para
  `/ws/input`; el clipboard aún sin auth). Pendiente extender auth al clipboard.
- Validar tamaño máximo de imagen. ✅ Hecho (10 MiB, responde 413).
- Manejo de errores de red y reconexión automática: el polling de Android ya
  reintenta solo; falta backoff y notificación de estado.
- El server no usa `http.MaxBytesReader` (el límite actual lee con
  `io.LimitReader`, que no cierra la conexión early). Menor.

### 6. Mejoras de UX
- Notificación de estado de conexión.
- Botón manual "Sincronizar ahora".
- Historial de clipboard (opcional).

## Cómo continuar en otro equipo

1. Clonar: `git clone https://github.com/Scag95/Clipboard_Share.git`
2. Servidor: `cd Clipboard_Share && go mod tidy && go run ./cmd/server`
3. Android: abrir `android/` en Android Studio, sincronizar Gradle, ejecutar en dispositivo.
4. Crear rama nueva por feature: `git checkout -b feat/<nombre>`.
5. Hacer PR a `main` y mergear.

## Notas técnicas

- El servidor escucha en `:8765` (env `CLIPBOARD_SHARE_PORT`).
- mDNS service type: `_clipboard-share._tcp`.
- Endpoints: `GET/POST /clipboard/text`, `GET/POST /clipboard/image`,
  `GET /health`, `POST /pair`, `GET /ws/input?token=...` (rama
  `feat/input-control`).
- Firewall de Windows debe permitir el puerto 8765.
- PC y móvil deben estar en la misma LAN para mDNS.
- Feature en curso mouse/teclado remoto: spec, tasks y evidencia en
  `odd/tasks/input-control.md`.
