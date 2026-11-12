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

### Servidor (PC / Windows)

- Go 1.22 o superior (se usa 1.24 por dependencias).
- Windows 10/11 (acceso al clipboard vía `golang.design/x/clipboard`).

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
go run ./cmd/server
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
- `POST /pair` — pairing por PIN: body `{"pin":"123456"}` (el PIN de 6 dígitos
  que el server imprime al arrancar). Devuelve `200` con `{"token":"..."}` si
  es correcto, `401` si es incorrecto o ya no vale, `400` si el body está mal.
  Tras 5 intentos seguidos fallidos el PIN se rota y se imprime uno nuevo.
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

El server puede recibir input del teléfono (rama `feat/input-control`,
especificación en `odd/tasks/input-control.md`):

1. Arrancar el server; anotar el **Pairing PIN** que imprime en la consola.
2. En el teléfono, ingresar el PIN → se obtiene un token de sesión (`/pair`).
3. La app abre `/ws/input` con el token y envía eventos (touchpad + teclado).

En Windows la inyección usa `SendInput` (sin cgo, build tag `windows`); en
otros sistemas el server registra eventos en un stub, útil para desarrollo.
El clipboard sigue sin autenticación por ahora; el control de input **no**.

## Generar binario

```bash
go build -o clipboard-share.exe ./cmd/server
```

## Estado del proyecto

- Servidor Go con clipboard real de Windows (texto + imagen), apagado ordenado
  (graceful shutdown) y límites de tamaño en la API.
- Control remoto mouse + teclado en rama `feat/input-control`: pairing por PIN,
  stream WebSocket con validación estricta, límites de tasa y `SendInput` en
  Windows (texto unicode, teclas nombradas, shortcuts con modificadores).
- Cliente Android: descubrimiento mDNS (NSD), sincronización bidireccional de
  texto e imágenes (PNG) con listener del clipboard y polling cada 2 s.
- Tests: handlers HTTP del server, broker de pairing, parser de eventos y
  stream WebSocket (`internal/api`, `internal/pair`, `internal/input`).
