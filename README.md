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
- `GET  /clipboard/text` — texto actual del clipboard.
- `POST /clipboard/text` — escribe texto al clipboard (`{"text":"..."}`).
- `GET  /clipboard/image` — imagen actual del clipboard (PNG).
- `POST /clipboard/image` — escribe imagen al clipboard (body binario).

## Generar binario

```bash
go build -o clipboard-share.exe ./cmd/server
```

## Estado del proyecto

- Servidor Go funcional con clipboard real de Windows (texto + imagen).
- Cliente Android: scaffold inicial (pendiente implementación).
EOF