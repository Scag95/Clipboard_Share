# Nudge

(Antes se llamaba Clipboard Share; ver “Migración desde clipboard-share”.)

Compartir el clipboard (texto e imágenes) entre un PC con Windows o Linux y un
móvil Android en la misma red WiFi, mediante descubrimiento automático
(mDNS/Bonjour) y transferencia P2P sobre HTTP local. Sin cables ni servicios en
la nube. El móvil además puede manejar el mouse y el teclado del PC.

## Objetivo

- PC (Windows o Linux) y móvil (Android) en la misma LAN.
- Copiar en uno → pegar en el otro, también con la app del móvil cerrada.
- Soporta texto e imágenes.
- El móvil controla mouse y teclado del PC, previa aprobación en el PC.

## Arquitectura

- **Servidor en el PC (Go)**: expone una API HTTP local y se anuncia vía mDNS
  (`_nudge._tcp`) para que el móvil lo descubra automáticamente.
- **Cliente Android**: descubre el servidor por mDNS, sincroniza el clipboard y
  hace de trackpad/teclado remoto.
- **Panel de escritorio (Linux, Qt Quick)**: ventana + bandeja para aprobar
  teléfonos y revocar dispositivos; habla con el servidor por un socket de
  control local.

## Estructura

```
Clipboard_Share/
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
├── odd/tasks/         # especificaciones de cada funcionalidad
└── README.md
```

## Requisitos

### Servidor (PC)

- Go 1.22 o superior (se usa 1.24 por dependencias).
- Windows 10/11 (acceso al clipboard vía `golang.design/x/clipboard`).
- **Linux**: X11 usa la misma librería; en Wayland el server usa
  `wl-copy`/`wl-paste` (`wl-clipboard`) automáticamente si hay
  `WAYLAND_DISPLAY`. Para el control remoto hace falta `/dev/uinput`
  (ver “Permisos uinput” más abajo); sin él, el server sigue corriendo y
  el clipboard funciona, pero la inyección de input queda en modo
  registro.
- **Panel de escritorio (opcional)**: Qt 6 (`qt6-base`, `qt6-declarative`,
  `qt6-wayland`), `qmake6` y `make` para compilarlo.

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

## App de escritorio para Linux (panel + servicio)

En Arch/Omarchy el servidor se instala como paquete `nudge`: trae el servidor
y CLI (`nudge`), un **panel gráfico** (`nudge-panel`) con ícono en la bandeja,
el servicio de usuario `nudge.service`, el ícono y la entrada del launcher
(`Super + Space` → “Nudge”).

```bash
cd pkgbuild && makepkg -f          # compila servidor + panel desde este árbol
sudo pacman -U nudge-*-x86_64.pkg.tar.zst
nudge-panel                        # la primera vez activa el servidor al iniciar sesión
```

`makepkg` necesita `go` como paquete de pacman; si Go lo tenés solo con `mise`,
usá `mise exec go@1.24 -- makepkg -f --nodeps` (salta únicamente el chequeo de
esa dependencia de compilación). Para cambiar el puerto:
`systemctl --user edit nudge` y definí `Environment=NUDGE_PORT=...` (los
nombres viejos `CLIPBOARD_SHARE_*` siguen valiendo como respaldo).

### Inicio automático

En el panel, la tarjeta **Ajustes** tiene dos interruptores independientes,
los dos encendidos después de instalar:

- **Iniciar el servidor al iniciar sesión**: `systemctl --user enable` /
  `disable nudge.service`. El panel lo activa una sola vez, en su primer
  arranque; si lo apagás, queda apagado.
- **Abrir el panel al iniciar sesión**: autostart XDG. Apagarlo escribe
  `~/.config/autostart/nudge-panel.desktop` con `Hidden=true`, que anula la
  entrada del paquete (`/etc/xdg/autostart/nudge-panel.desktop`); encenderlo
  borra ese archivo.

Abrir el panel siempre arranca el servidor si no está corriendo
(`systemctl --user start nudge.service`), diga lo que diga el primer
interruptor. Si un cambio falla, la tarjeta muestra el motivo.

### Migración desde clipboard-share

El paquete `nudge` reemplaza a `clipboard-share` (`replaces`/`conflicts`/
`provides`). Antes de actualizar, apagá el servicio viejo:
`systemctl --user disable --now clipboard-share`. Al primer arranque, el
**servidor** mueve `~/.config/clipboard-share` a `~/.config/nudge` (si este
último no existe todavía), así los teléfonos emparejados siguen funcionando;
un `~/.config/nudge` existente nunca se sobrescribe. El CLI y el panel no
mueven nada: usan `~/.config/nudge` si existe y, si no, el directorio viejo.
Si el servidor no puede moverlo, sigue trabajando con el directorio viejo y
no se pierde ningún emparejamiento. Durante la transición el servidor anuncia
el servicio mDNS nuevo (`_nudge._tcp`) y el viejo (`_clipboard-share._tcp`), y
la app de Android busca los dos.

El panel (`nudge-panel`) arranca solo con la sesión, oculto en la
bandeja; cuando un teléfono pide emparejarse, **la bandeja avisa** (clic en el
aviso o en el ícono abre el panel). La ventana nunca se abre sola ni toma foco:
cualquiera en la LAN puede mandar una solicitud, y aprobar es siempre un clic
deliberado en el botón:

- **Aprobar / Rechazar** cada solicitud (caducan a los 90 s).
- **Revocar** un dispositivo emparejado (pide confirmación).
- Estado del servidor; si no corre, lo dice y sugiere
  `systemctl --user start nudge`.
- Teclas: `?` ayuda, `Esc` oculta a la bandeja, `Q` sale. Clic en el ícono de
  la bandeja muestra/oculta la ventana. No hay atajo de teclado para aprobar.

El paquete también instala una regla udev para `/dev/uinput` (ver “Permisos
uinput”). Detalles del paquete en `pkgbuild/README.md`. Sin paquete, se puede
probar el panel a mano: `panel/bin/build && panel/build/nudge-panel`;
sus tests corren con `panel/bin/test` (Qt Test, offscreen).

## Uso (servidor PC)

```bash
go mod tidy
go run ./cmd/server   # o el binario, ver “Generar binario”
```

### Permisos uinput (Linux/Wayland)

Wayland prohíbe la inyección global de input por protocolo; el server usa
`/dev/uinput`. En Arch el nodo ya es `root:input` con modo `0660`, así que
alcanza con que tu usuario esté en el grupo `input`:

```bash
sudo usermod -aG input "$USER"   # y volver a iniciar sesión
```

El log de arranque dice qué modo está activo (`input: uinput injector active`
o el stub de registro). El paquete instala además una regla con `uaccess`
(`pkgbuild/99-nudge-uinput.rules`) pensada para dar acceso al usuario
con sesión activa sin el grupo; **no está verificada**: en la máquina de
desarrollo el ACL de `/dev/uinput` no mostró ninguna entrada para el usuario y
el acceso venía del grupo `input`.

El servidor escucha en `:8765` (configurable con `NUDGE_PORT`; también
`CLIPBOARD_SHARE_PORT` como respaldo) y se anuncia en la red local como
`_nudge._tcp`.

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

1. En el teléfono, elegir el host de la lista de inicio: abre el trackpad y
   pide el emparejamiento si es la primera vez.
2. En el PC aparece una notificación y el **panel** muestra la solicitud:
   tocar **Aprobar**. El CLI sigue disponible para scripts:

   ```bash
   nudge pair --last   # aprueba el request más nuevo
   nudge pair list     # ver pendientes y aprobados
   nudge pair revoke --device <id>   # quitar un dispositivo
   ```

3. El celu guarda el token y abre `/ws/input`. **Las reconexiones siguientes
   son automáticas**: el server reconoce el UUID del dispositivo y re-emite
   token sin aprobación; el estado (dispositivos y tokens) persiste en
   `~/.config/nudge/state.json` (`0600`) y sobrevive reinicios del
   server y de la PC.

Atajo opcional para Hyprland (`~/.config/hypr/hyprland.conf`) que aprueba sin
abrir el panel:

```ini
bind = SUPER SHIFT, P, exec, nudge pair --last
```

Si se pierde un teléfono: **Revocar** en el panel (o
`nudge pair revoke --device <id>`); para resetear todo el pairing,
borrar `state.json`.

En Windows la inyección usa `SendInput` (sin cgo, build tag `windows`); en
Linux usa `/dev/uinput` (requiere el permiso de la sección “Permisos
uinput”; sin acceso, el server registra eventos en un stub, útil para
desarrollo). **El clipboard sigue sin autenticación**: cualquiera en la LAN
puede leerlo y escribirlo; el control de input **no** (requiere token). Una
solicitud de emparejamiento puede mandarla cualquiera en la LAN, por eso el
panel nunca la aprueba solo ni abre su ventana por ella.

## Generar binario

```bash
# Windows (desde Linux, sin cgo):
GOOS=windows go build -o nudge.exe ./cmd/server

# Linux nativo:
go build -o nudge ./cmd/server
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
sin Play Store. El control remoto requiere la pantalla abierta.

### Sync de clipboard en segundo plano (celu → PC)

Al elegir un host arranca un servicio con notificación persistente (requisito
de Android) que mantiene **PC → celu** en segundo plano sin configurar nada.
Para el sentido contrario, **celu → PC con la app cerrada**, hay que darle dos
permisos que Android no deja pedir desde la interfaz (una vez, con el celu
conectado por `adb`):

```bash
adb shell pm grant com.clipboardshare android.permission.READ_LOGS
adb shell appops set com.clipboardshare SYSTEM_ALERT_WINDOW allow
```

Además Android muestra, al abrir la app, el diálogo **"¿Permitir que Nudge
acceda a todos los registros del dispositivo?"**: elegí *Permitir acceso
por única vez*. Es un consentimiento tuyo sobre datos sensibles y solo puede
aceptarse con la app abierta; si el sistema lo vuelve a pedir (reinicio del
celu o del proceso), aceptalo de nuevo.

Cómo funciona: Android niega el clipboard a las apps sin foco, pero registra en
el log del sistema cada cambio dirigido a una app sin foco. El servicio lee esa
línea (solo la de su propio paquete), abre una fracción de segundo una ventana
transparente que gana foco, lee el clipboard, lo envía a la PC y la cierra.
Mientras tanto retiene el clipboard de la PC para que tu copia gane. Sin los
permisos, el estado de la pantalla remota lo indica ("Falta permiso READ_LOGS:
ver README") y el servicio sigue sincronizando solo PC → celu. Si los permisos
están pero pasan 60 s sin que llegue ninguna línea del log, el estado agrega
"Sin datos del log del sistema": suele ser el diálogo de acceso a registros sin
aceptar (también puede aparecer si simplemente no hubo copias en ese tiempo).
Los logs de la app nunca incluyen el contenido copiado: solo el tipo y el
tamaño (`<text 9 chars>`).

Es frágil por diseño: depende del texto de un log del sistema, que Android puede
cambiar entre versiones. Probado en Android 16 (Samsung S24), con el servicio
del paquete corriendo en el PC. Una escritura propia en segundo plano genera
**dos** líneas de log (~340 ms entre sí), y el servicio las trata como eco
propio para no abrir la ventana de lectura tras cada copia que llega de la PC.

## Estado del proyecto

- Servidor Go con clipboard real de Windows **y Linux** (texto + imagen; en
  Wayland vía `wl-clipboard`), apagado ordenado (graceful shutdown) y límites
  de tamaño en la API.
- Control remoto mouse + teclado: pairing por identidad de dispositivo con
  aprobación en el PC (sin PIN), estado persistido, stream WebSocket con
  validación estricta y límites de tasa; `SendInput` en Windows y `uinput` en
  Linux.
- Panel de escritorio para Linux (Qt Quick) con bandeja, paquete Arch `nudge`,
  servicio de usuario, interruptores de inicio automático (servidor y panel),
  ícono y entrada de launcher. Verificado instalando
  el paquete: el servicio arranca, se reinicia solo si cae y el panel lo ve.
- Cliente Android: home con lista de hosts (vivos + recordados), tokens por
  host con reconexión transparente, sincronización bidireccional de texto e
  imágenes (PNG) y control remoto (trackpad + teclado + botones de click) en su
  propia pantalla. El sync corre en segundo plano vía ForegroundService atado
  al último host elegido: PC → celu sin configuración, y celu → PC con la app
  cerrada tras dar dos permisos por `adb` (ver "Sync de clipboard en segundo
  plano").
- Tests:
  - Go: `go test ./...` (`internal/api`, `internal/clipboard`, `internal/input`,
    `internal/pair`).
  - Panel: `panel/bin/test` (Qt Test offscreen: backend contra un servidor
    falso, UI con clics y teclas reales, instancia única, inicio automático
    con un `systemctl` falso y directorios temporales).
  - Android: `cd android && ./gradlew testDebugUnitTest` (JUnit sobre la lógica
    de disparo del sync en segundo plano).
- Conocido y sin resolver: el clipboard HTTP no está autenticado; el sync celu →
  PC depende del texto de un log del sistema de Android y de un diálogo de
  consentimiento que solo el usuario puede aceptar; la regla udev `uaccess` no
  está verificada. Los hallazgos no bloqueantes de los reviews están en
  `odd/tasks/review-followups.md`.
