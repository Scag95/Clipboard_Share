# Changelog

Los cambios de cada versión de Nudge. El formato sigue
[Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y las versiones,
[Semantic Versioning](https://semver.org/lang/es/).

## [1.0.0] - sin publicar

La 1.0 cifra todo el tráfico, exige el token también para el portapapeles y
cambia el id de la app de Android. No es compatible con la 0.1.0: leé
[Para actualizar desde 0.1.0](#para-actualizar-desde-010) antes de instalarla.

### Agregado

- Windows: ícono en el área de notificación. Con clic derecho muestra el
  estado, el código de verificación de la PC y los teléfonos emparejados. Una
  solicitud de emparejamiento también avisa con una notificación; aprobar
  sigue siendo solo desde la ventana Sí/No.
- Windows: `nudge.exe` y `nudge-background.exe` traen ícono, información de
  versión y un manifiesto.
- Versión del protocolo (#12). `/health` responde
  `{"status":"ok","protocol":1,"version":"1.0.0"}`, y el anuncio mDNS lleva
  `protocol=1`. Si la app y la PC no hablan el mismo protocolo, la app no se
  conecta y dice qué lado actualizar ("La PC usa otra versión de Nudge…").
- Android: el APK de release se firma con la clave del proyecto, tomada de
  variables de entorno (`NUDGE_KEYSTORE` y sus contraseñas). Sin ellas sale
  sin firmar; nunca se usa la clave debug (#14).
- Workflow de release (#15): al pushear un tag `v*` compila el paquete de
  Arch, el zip de Windows y el APK firmado, genera `SHA256SUMS` y publica la
  release con las notas de este archivo.
- Workflow `windows.yml`: corre los tests de Go en `windows-latest` y prueba
  el zip de punta a punta (instalador, servidor, CLI, emparejamiento,
  portapapeles y desinstalador).
- Este `CHANGELOG.md` y [docs/faq.md](docs/faq.md) con los problemas
  frecuentes (#17).

### Cambiado

- Android: la app pasa a ser `com.scag95.nudge` (antes `com.clipboardshare`),
  versión 1.0.0. Para Android es otra app, así que la 0.1.0 hay que
  desinstalarla (#14).
- `go test ./...` ya no toca el portapapeles real: esos tests corren solo con
  `NUDGE_CLIPBOARD_TESTS=1` (#16).
- La ventana de emparejamiento de Windows habla en voseo, como el instalador:
  "Si no reconocés este dispositivo, elegí No."
- El README ahora presenta el proyecto; el detalle técnico pasó a
  [docs/linux.md](docs/linux.md), [docs/windows.md](docs/windows.md),
  [docs/android.md](docs/android.md) y
  [docs/development.md](docs/development.md).

### Quitado

- La compatibilidad con Clipboard Share (#13). El servidor ya no lee las
  variables `CLIPBOARD_SHARE_*`, no usa ni migra `~/.config/clipboard-share`
  y solo se anuncia como `_nudge._tcp`; la app solo busca `_nudge._tcp`. El
  paquete de Arch ya no declara `provides`/`conflicts`/`replaces` de
  `clipboard-share`.
- HTTP plano: el servidor solo atiende HTTPS y WSS.
- `PLAN.md` y `android/README.md`, que estaban desactualizados (el segundo lo
  reemplaza [docs/android.md](docs/android.md)).

### Corregido

- Linux: el control remoto no movía nada desde el cambio de nombre a Nudge.
  La estructura `uinput_setup` tenía los campos en otro orden que el kernel,
  `UI_DEV_SETUP` fallaba con `EINVAL` y el servidor quedaba en modo registro
  sin avisar. Un test fija ahora el layout contra `linux/uinput.h`.
- Windows: `Uninstall Nudge.cmd` no encontraba `uninstall.ps1` si se lo
  abría con una ruta relativa.
- Android: un `401` de un pedido hecho con un token viejo ya no corta el
  sync ni pide emparejar de nuevo cuando ya hay un token nuevo guardado.

### Seguridad

- El portapapeles exige el token del teléfono (#10). `/clipboard/*` responde
  `401` sin `Authorization: Bearer <token>` y no lee ni escribe nada. Si la
  app recibe un `401`, borra el token y pide emparejar de nuevo.
- Todo el tráfico va cifrado (#11). El servidor genera un certificado
  autofirmado (`tls.crt` y `tls.key`, junto a `state.json`) y atiende HTTPS y
  WSS con TLS 1.2 o superior. Al emparejar, la app y la PC muestran el mismo
  código de verificación (`XXXX-XXXX-XXXX`); la PC lo muestra en el log, en
  `nudge pair list`, en el panel y la notificación de Linux, y en la ventana
  Sí/No y el ícono de la bandeja de Windows. La app guarda la huella del
  certificado y después rechaza cualquier otro: si cambia, avisa "El
  certificado de la PC cambió. Emparejala de nuevo."
- Si una PC sin pin guardado da acceso sin pedir aprobación, la app pregunta
  "¿La PC muestra el código …?" antes de guardarla; con **No** no guarda
  nada.
- Un teléfono conocido recibe acceso sin aprobación solo si presenta su token
  vigente (#21). Conocer su id ya no alcanza: sin el token, la PC vuelve a
  pedir aprobación con el código, como para un teléfono nuevo.

### Para actualizar desde 0.1.0

> **La app vieja y la nueva no se hablan, y la configuración de Clipboard
> Share ya no se migra.**
>
> 1. Desinstalá la app 0.1.0 del teléfono (era `com.clipboardshare`; con
>    `adb`: `adb uninstall com.clipboardshare`).
> 2. Instalá Nudge 1.0.0 en la PC y en el teléfono. En Linux,
>    `sudo pacman -U nudge-1.0.0-1-x86_64.pkg.tar.zst`; en Windows, volvé a
>    ejecutar `Install Nudge.cmd`, que actualiza en el lugar.
> 3. Emparejá el teléfono otra vez, comparando el código de verificación. El
>    teléfono viejo sigue en la lista de la PC: podés quitarlo con
>    `nudge pair revoke --device <id>` (el id sale en `nudge pair list`).
>
> Si todavía tenés la configuración en `~/.config/clipboard-share` (nunca
> corriste el servidor 0.1.0), la 1.0 no la lee: empieza de cero en
> `~/.config/nudge`. Las variables `CLIPBOARD_SHARE_*` tampoco se leen; usá
> `NUDGE_PORT`.

## [0.1.0] - 2026-11-18

Primera pre-release con el nombre Nudge (antes Clipboard Share).

- Portapapeles en los dos sentidos entre Android y la PC, con texto e
  imágenes. PC → celu funciona en segundo plano; celu → PC con la app
  cerrada, después de dar dos permisos por `adb`.
- Mouse y teclado remotos desde el celu: trackpad con franja de scroll,
  teclado y botones de clic. Usa `/dev/uinput` en Linux y `SendInput` en
  Windows.
- Emparejamiento por identidad del teléfono, aprobado en la PC, sin PIN. Los
  tokens quedan guardados y el celu se reconecta solo.
- Linux: servidor y CLI `nudge`, panel `nudge-panel` con ícono en la bandeja
  e interruptores de inicio automático, y paquete de Arch `nudge` con el
  servicio de usuario, la regla udev y la entrada del launcher. Al primer
  arranque el servidor movía `~/.config/clipboard-share` a `~/.config/nudge`.
- Windows 10/11 x64: zip con instalador por usuario, compilado desde Linux y
  sin probar en Windows.
- Android: APK con firma debug (`com.clipboardshare`), lista de PCs con
  descubrimiento por mDNS.
- Licencia MIT.

En esta versión el portapapeles no pedía token y el tráfico iba sin cifrar;
los dos puntos se resolvieron en la 1.0.0.

[1.0.0]: https://github.com/Scag95/Nudge/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/Scag95/Nudge/releases/tag/v0.1.0
