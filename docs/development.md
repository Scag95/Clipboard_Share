# Desarrollo

Cómo está armado Nudge por dentro, cómo compilarlo y cómo hablarle al
servidor. Para instalar y usar, ver [linux.md](linux.md),
[windows.md](windows.md) y [android.md](android.md).

## Arquitectura

- **Servidor en el PC (Go)**: expone una API HTTP local y se anuncia vía mDNS
  (`_nudge._tcp`) para que el móvil lo descubra automáticamente.
- **Cliente Android**: descubre el servidor por mDNS, sincroniza el clipboard y
  hace de trackpad/teclado remoto.
- **Panel de escritorio (Linux, Qt Quick)**: ventana + bandeja para aprobar
  teléfonos y revocar dispositivos; habla con el servidor por un socket de
  control local.

La inyección de input usa `SendInput` en Windows (sin cgo, build tag
`windows`) y `/dev/uinput` en Linux (ver
[Permisos de /dev/uinput](linux.md#permisos-de-devuinput)). Sin acceso a
uinput, el servidor registra los eventos en un stub, útil para desarrollo.

## Estructura

```
Nudge/
├── cmd/server/        # entrypoint del servidor Go (y el CLI `pair`)
├── internal/
│   ├── api/           # handlers HTTP y WebSocket de control remoto
│   ├── clipboard/     # clipboard del SO (Windows, X11, Wayland)
│   ├── discovery/     # mDNS announce/browse
│   ├── input/         # inyección de mouse/teclado (SendInput, uinput)
│   └── pair/          # emparejamiento por identidad + socket de control
├── android/           # app Android (Kotlin)
├── panel/             # panel de escritorio Linux (C++ / Qt Quick) y sus tests
├── pkgbuild/          # paquete Arch `nudge`, servicio de usuario, launcher, ícono
├── packaging/windows/ # instalador por usuario, scripts y recursos del .exe
├── scripts/           # build-windows.sh: zip de Windows
├── docs/              # documentación por plataforma y de desarrollo
└── README.md
```

## Requisitos del servidor

- Go 1.22 o superior (se usa 1.24 por dependencias).
- Windows 10/11: acceso al clipboard vía `golang.design/x/clipboard`.
- Linux: ver [Requisitos en Linux](linux.md#requisitos).

Dependencias Go (se descargan con `go mod tidy`):

- `github.com/gorilla/mux` v1.8.1: router HTTP.
- `github.com/gorilla/websocket` v1.5.3: stream de eventos de control remoto.
- `github.com/grandcat/zeroconf` v1.0.0: anuncio mDNS.
- `golang.design/x/clipboard` v0.11.0: lectura/escritura del clipboard del SO.

Los requisitos de la app están en [android.md](android.md#generar-el-apk).

## Correr el servidor desde el código

```bash
go mod tidy
go run ./cmd/server   # o el binario, ver "Compilar los binarios"
```

El servidor escucha en `:8765` (configurable con `NUDGE_PORT`; también
`CLIPBOARD_SHARE_PORT` como respaldo) y se anuncia en la red local como
`_nudge._tcp`.

## Compilar los binarios

```bash
# Windows (desde Linux, sin cgo); el zip completo sale de scripts/build-windows.sh:
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o nudge.exe ./cmd/server

# Linux nativo:
go build -o nudge ./cmd/server
```

El zip de Windows se explica en
[Recompilar el zip](windows.md#recompilar-el-zip), el paquete de Arch en
[Instalar el paquete](linux.md#instalar-el-paquete) y el APK en
[Generar el APK](android.md#generar-el-apk).

## API HTTP

- `GET  /health`: estado del servidor.
- `GET  /clipboard/text`: texto actual del clipboard (`{"text":""}` si no hay
  texto).
- `POST /clipboard/text`: escribe texto al clipboard (`{"text":"..."}`).
- `GET  /clipboard/image`: imagen actual del clipboard (PNG); `204` si no hay
  imagen.
- `POST /clipboard/image`: escribe imagen al clipboard (body binario PNG).
  Tamaño máximo: 10 MiB; si se excede responde `413` sin tocar el clipboard.
- `POST /pair/request`: body `{"id":"<uuid dispositivo>","name":"<modelo>"}`.
  Dispositivo ya aprobado → `200 {"token":"..."}` inmediato (reconexión
  transparente, el token anterior se revoca). Desconocido → `202
  {"requestId":"..."}` y notificación en el PC para aprobar; el request
  caduca a los 90 s. Más de 5 requests pendientes de la misma IP → `429`.
- `GET  /pair/status?id=<requestId>`: `pending`; al aprobar, `200
  {"state":"approved","token":"..."}` **una sola vez**; si se denegó, `200
  denied`; si venció, `410 expired`.
- `GET  /ws/input?token=<token>`: WebSocket de control remoto (ver abajo).

## Protocolo de control remoto

`/ws/input` sin token válido responde `401` antes del upgrade. Los eventos se
envían como objetos JSON, uno por frame:

```jsonc
{ "type": "move",   "dx": 12, "dy": -5 }              // delta relativo (≤1000)
{ "type": "click",  "button": "left",  "mode": "tap" } // tap | down | up
{ "type": "scroll", "dx": 0,  "dy": 2 }               // notch (≤100)
{ "type": "key",    "key": "c", "mods": ["ctrl"] }    // tecla nombrada o carácter
{ "type": "text",   "chars": "hola ñ" }               // unicode (≤1000 chars)
```

Un evento inválido o fuera de rango responde `{"error": "..."}` y **no se
inyecta**; la conexión sigue abierta. Máximo 120 eventos/s por conexión.

## Emparejamiento por dentro

El celu guarda el token y abre `/ws/input`. Las reconexiones siguientes son
automáticas: el servidor reconoce el UUID del dispositivo y re-emite el token
sin aprobación. El estado (dispositivos y tokens) persiste en
`~/.config/nudge/state.json` (`0600`; en Windows `%APPDATA%\nudge\state.json`)
y sobrevive reinicios del servidor y del PC.

## Seguridad

- **El clipboard HTTP no tiene autenticación**: cualquiera en la LAN puede
  leerlo y escribirlo.
- El control de input **sí** requiere token.
- No hay TLS: el servidor usa HTTP y WebSocket planos, así que el token y los
  eventos viajan sin cifrar por la LAN.
- Una solicitud de emparejamiento puede mandarla cualquiera en la LAN; por eso
  el panel nunca la aprueba solo ni abre su ventana por ella.

## Tests

- Go: `go test ./...` (`internal/api`, `internal/clipboard`, `internal/input`,
  `internal/pair`).
- Panel: `panel/bin/test` (Qt Test offscreen: backend contra un servidor
  falso, UI con clics y teclas reales, instancia única, inicio automático con
  un `systemctl` falso y directorios temporales).
- Android: `cd android && ./gradlew testDebugUnitTest` (JUnit sobre la lógica
  de disparo del sync en segundo plano).
- Windows: el workflow [`windows.yml`](../.github/workflows/windows.yml) corre
  la suite de Go en `windows-latest` y prueba el zip de punta a punta.

## Estado del proyecto

- Servidor Go con clipboard real de Linux (texto + imagen; en Wayland vía
  `wl-clipboard`), apagado ordenado (graceful shutdown) y límites de tamaño en
  la API. El build de Windows se prueba en GitHub Actions, no en una PC
  física.
- Control remoto mouse + teclado: pairing por identidad de dispositivo con
  aprobación en el PC (sin PIN), estado persistido, stream WebSocket con
  validación estricta y límites de tasa; `uinput` en Linux y `SendInput` en
  Windows.
- Panel de escritorio para Linux (Qt Quick) con bandeja, paquete Arch `nudge`,
  servicio de usuario, interruptores de inicio automático (servidor y panel),
  ícono y entrada de launcher. Verificado instalando el paquete: el servicio
  arranca, se reinicia solo si cae y el panel lo ve.
- Cliente Android: home con lista de hosts (vivos + recordados), tokens por
  host con reconexión transparente, sincronización bidireccional de texto e
  imágenes (PNG) y control remoto (trackpad + teclado + botones de click) en
  su propia pantalla. El sync corre en segundo plano vía ForegroundService
  atado al último host elegido: PC → celu sin configuración, y celu → PC con
  la app cerrada tras dar dos permisos por `adb` (ver
  [Sync en segundo plano](android.md#sync-de-clipboard-en-segundo-plano-celu--pc)).

### Conocido y sin resolver

- El clipboard HTTP no está autenticado.
- El sync celu → PC depende del texto de un log del sistema de Android y de un
  diálogo de consentimiento que solo el usuario puede aceptar.
- La regla udev `uaccess` no está verificada.
