# Clipboard Share

Compartir el clipboard (texto e imágenes) entre un PC con Windows y un móvil Android
en la misma red WiFi, mediante descubrimiento automático (mDNS/Bonjour) y
transferencia P2P sobre HTTP local.

## Objetivo

- PC (Windows) y móvil (Android) en la misma LAN.
- Copiar en uno → pegar en el otro, sin cables ni servicios en la nube.
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

## Uso (servidor PC)

```bash
go mod tidy
go run ./cmd/server
```

El servidor escucha en `:8765` y se anuncia en la red local.
EOF