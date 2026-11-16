# Clipboard Share

Compartir el clipboard (texto e imágenes) entre un PC con Windows y un móvil
Android en la misma red WiFi, mediante descubrimiento automático (mDNS/Bonjour)
y transferencia P2P sobre HTTP local. Sin cables ni servicios en la nube.

## Objetivo

- PC (Windows) y móvil (Android) en la misma LAN.
- Copiar en uno → pegar en el otro.
- Soporta texto e imágenes.

## Arquitectura

- **Servidor en el PC (Go)**: expone una API HTTP local y se anuncia vía mDNS
  (`_clipboard-share._tcp`) para que el móvil lo descubra automáticamente.
- **Cliente Android**: descubre el servidor por mDNS y sincroniza el clipboard.

## Estructura

```
Clipboard_Share/
├── cmd/server/        # entrypoint del servidor Go
├── internal/
│   ├── api/           # handlers HTTP
│   ├── clipboard/     # acceso al clipboard del SO (Windows)
│   └── discovery/     # mDNS announce/browse
├── android/           # proyecto Android (Kotlin)
└── README.md
```

## Requisitos

### Servidor (PC)

- Go 1.22 o superior (se usa 1.24 por dependencias).
- Windows 10/11 (acceso al clipboard vía `golang.design/x/clipboard`).
- **Linux (beta)**: X11 usa la misma librería; en Wayland el server usa
  `wl-copy`/`wl-paste` (`wl-clipboard`) automáticamente si hay
  `WAYLAND_DISPLAY`. Para el control remoto hace falta `/dev/uinput`
  (ver “Permisos uinput” más abajo); sin él, el server sigue corriendo y
    el clipboard funciona, pero la inyección de input queda en modo
    registro.

Dependencias Go (se descargan con `go mod tidy`):

- `github.com/gorilla/mux` v1.8.1 — router HTTP.
- `github.com/gorilla/websocket` v1.5.3 — stream de eventos de control remoto.
- `github.com/grandcat/zeroconf` v1.0.0 — anuncio mDNS.
- `golang.design/x/clipboard` v0.11.0 — lectura/escritura del clipboard del SO.

### Cliente (Android)

- Android Studio (Hedgehog o superior).
- JDK 17.
- Android SDK 24+ (minSdk).
- Gradle 8+.
- Dependencias previstas:
  - Kotlin stdlib
  - AndroidX (AppCompat, Core KTX)
  - OkHttp / Retrofit — cliente HTTP
  - `javax.jmdns` o NSD nativo (`NsdManager`) — descubrimiento mDNS
  - Coroutines — operaciones asíncronas

## Uso (servidor PC)

```bash
go mod tidy
go run ./cmd/server   # o el binario, ver “Generar binario”
```

### Permisos uinput (Linux/Wayland)

Wayland prohíbe la inyección global de input por protocolo; el server usa
`/dev/uinput`, que por defecto es solo de root. Para permitir que tu
usuario cree dispositivos virtuales (necesita `sudo` una sola vez):

```bash
sudo tee /etc/udev/rules.d/99-clipboard-share-uinput.rules >/dev/null <<'EOF'
KERNEL=="uinput", GROUP="input", MODE="0660"
EOF
sudo usermod -aG input "$USER"
# recargar (o reiniciar):
sudo udevadm control --reload && sudo udevadm trigger
```

El servidor escucha en `:8765` (configurable con `CLIPBOARD_SHARE_PORT`) y se
anuncia en la red local como `_clipboard-share._tcp`.

Endpoints:

- `GET  /health` — estado del servidor.
- `GET  /clipboard/text` — texto actual del clipboard (`{"text":""}` si no hay texto).
- `POST /clipboard/text` — escribe texto al clipboard (`{"text":"..."}`).
- `GET  /clipboard/image` — imagen actual del clipboard (PNG); `204` si no hay imagen.
- `POST /clipboard/image` — escribe imagen al clipboard (body binario PNG).
  Tamaño máximo: 10 MiB; si se excede responde `413` sin tocar el clipboard.
- `POST /pair/request` — body `{"id":"<uuid dispositivo>","name":"<modelo>"}`.
  Dispositivo ya aprobado → `200 {"token":"..."}` inmediato (reconexión
  transparente, el token anterior se revoca). Desconocido → `202
  {"requestId":"..."}` y notificación en el PC para aprobar; el request
  caduca a los 90 s. Más de 5 requests pendientes de la misma IP → `429`.
- `GET  /pair/status?id=<requestId>` — `pending`; al aprobar, `200
  {"state":"approved","token":"..."}` **una sola vez**; si se denegó, `200
  denied`; si venció, `410 expired`.
- `GET  /ws/input?token=<token>` — WebSocket de control remoto. Sin token
  válido responde `401` antes del upgrade. Los eventos se envían como objetos
  JSON (uno por frame):

  ```jsonc
  { "type": "move",   "dx": 12, "dy": -5 }              // delta relativo (≤1000)
  { "type": "click",  "button": "left",  "mode": "tap" } // tap | down | up
  { "type": "scroll", "dx": 0,  "dy": 2 }               // notch (≤100)
  { "type": "key",    "key": "c", "mods": ["ctrl"] }    // tecla nombrada o carácter
  { "type": "text",   "chars": "hola ñ" }               // unicode (≤1000 chars)
  ```

  Un evento inválido o fuera de rango responde `{"error": "..."}` y **no se
  inyecta**; la conexión sigue abierta. Máximo 120 eventos/s por conexión.

## Control remoto (mouse + teclado)

El server puede recibir input del teléfono (especificación en
`odd/tasks/input-control.md` y `odd/tasks/transparent-pairing.md`):

1. En el teléfono, tocar **Conectar** en la pantalla de control remoto.
2. En el PC aparece una notificación; aprobar el dispositivo:

   ```bash
   clipboard-share pair --last   # aprueba el request más nuevo
   clipboard-share pair list     # ver pendientes y aprobados
   clipboard-share pair revoke --device <id>   # quitar un dispositivo
   ```

3. El celu guarda el token y abre `/ws/input`. **Las reconexiones siguientes
   son automáticas**: el server reconoce el UUID del dispositivo y re-emite
   token sin aprobación; el estado (dispositivos y tokens) persiste en
   `~/.config/clipboard-share/state.json` (`0600`) y sobrevive reinicios del
   server y de la PC.

Atajo recomendado para Hyprland (`~/.config/hypr/hyprland.conf`), que además
deja de depender de la notificación:

```ini
bind = SUPER SHIFT, P, exec, clipboard-share pair --last
```

Si se pierde un teléfono: `clipboard-share pair revoke --device <id>`; para
resetear todo el pairing, borrar `state.json`.

En Windows la inyección usa `SendInput` (sin cgo, build tag `windows`); en
Linux usa `/dev/uinput` (requiere el permiso de la sección “Permisos
uinput”; sin acceso, el server registra eventos en un stub, útil para
desarrollo; el log de arranque indica qué modo está activo). El clipboard
sigue sin autenticación por ahora; el control de input **no**.

## Generar binario

```bash
# Windows (desde Linux, sin cgo):
GOOS=windows go build -o clipboard-share.exe ./cmd/server

# Linux nativo:
go build -o clipboard-share ./cmd/server
```

## Generar APK (beta personal)

Requiere JDK 17 y Android SDK (platform 34 + build-tools 34). `android/`
tiene wrapper de Gradle, así que no hace falta tener Gradle instalado:

```bash
cd android
./gradlew assembleDebug
# salida: app/build/outputs/apk/debug/app-debug.apk (firma debug)
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

La beta es de uso personal: APK debug por USB/`adb` o `adb connect` por Wi-Fi,
sin Play Store. El sync de clipboard corre en segundo plano apenas elegís un
host (con notificación persistente, requisito de Android); el control remoto
requiere la pantalla abierta.

## Estado del proyecto

- Servidor Go con clipboard real de Windows **y Linux** (texto + imagen; en
  Wayland vía `wl-clipboard`), apagado ordenado (graceful shutdown) y límites
  de tamaño en la API.
- Control remoto mouse + teclado: pairing por identidad de dispositivo con
  aprobación en el PC (sin PIN), estado persistido, stream WebSocket con
  validación estricta y límites de tasa; `SendInput` en Windows y `uinput` en
  Linux.
- Cliente Android: home con lista de hosts (vivos + recordados), tokens por
  host con reconexión transparente, sincronización bidireccional de texto e
  imágenes (PNG) en **segundo plano** vía ForegroundService atado al último
  host elegido (tras reiniciar el teléfono alcanza con abrir la app una vez
  para reactivar el servicio), y control remoto (trackpad + teclado + botones
  de click) en su propia pantalla.
- Tests: handlers HTTP del server, broker de pairing, parser de eventos y
  stream WebSocket (`internal/api`, `internal/pair`, `internal/input`).
