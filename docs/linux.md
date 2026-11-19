# Nudge en Linux

Guía para instalar y usar Nudge en Linux. Está pensada para Arch/Omarchy, que
es donde se armó y se probó el paquete. Para compilar desde el código sin
paquete, ver [development.md](development.md).

## Requisitos

- **Clipboard**: en X11 el servidor usa `golang.design/x/clipboard`. En
  Wayland usa `wl-copy`/`wl-paste` (paquete `wl-clipboard`) automáticamente si
  hay `WAYLAND_DISPLAY`.
- **Control remoto**: hace falta acceso a `/dev/uinput` (ver
  [Permisos de /dev/uinput](#permisos-de-devuinput)). Sin él, el servidor
  sigue corriendo y el clipboard funciona, pero la inyección de input queda en
  modo registro.
- **Panel de escritorio** (opcional si compilás a mano): Qt 6 (`qt6-base`,
  `qt6-declarative`, `qt6-wayland`), `qmake6` y `make`.

## Instalar el paquete

En Arch/Omarchy Nudge se instala como paquete `nudge`. Trae:

- el servidor y el CLI (`nudge`);
- el panel gráfico (`nudge-panel`) con ícono en la bandeja;
- el servicio de usuario `nudge.service`;
- una regla udev para `/dev/uinput`;
- el ícono y la entrada del launcher (`Super + Space` → "Nudge").

Hay un paquete ya compilado en la
[página de releases](https://github.com/Scag95/Nudge/releases). Para
compilarlo desde este árbol:

```bash
cd pkgbuild && makepkg -f          # compila servidor + panel desde este árbol
sudo pacman -U nudge-*-x86_64.pkg.tar.zst
nudge-panel                        # la primera vez activa el servidor al iniciar sesión
```

`makepkg` necesita `go` como paquete de pacman. Si Go lo tenés solo con
`mise`, usá `mise exec go@1.24 -- makepkg -f --nodeps` (salta únicamente el
chequeo de esa dependencia de compilación).

La lista de archivos instalados y otros detalles del empaquetado están en
[pkgbuild/README.md](../pkgbuild/README.md).

## El panel

El panel (`nudge-panel`) arranca solo con la sesión, oculto en la bandeja.
Cuando un teléfono pide emparejarse, **la bandeja avisa**; un clic en el aviso
o en el ícono abre el panel. La ventana nunca se abre sola ni toma foco:
cualquiera en la LAN puede mandar una solicitud, así que aprobar es siempre un
clic deliberado en el botón.

Desde el panel podés:

- **Aprobar / Rechazar** cada solicitud (caducan a los 90 s). Arriba de las
  solicitudes se ve el código de verificación de la PC
  (`Verification code: XXXX-XXXX-XXXX`): aprobá solo si el teléfono muestra
  el mismo. El código está siempre visible mientras el servidor corre,
  también sin solicitudes pendientes.
- **Revocar** un dispositivo emparejado (pide confirmación).
- Ver el estado del servidor. Si no corre, el panel lo dice y sugiere
  `systemctl --user start nudge`.

Teclas: `?` ayuda, `Esc` oculta a la bandeja, `Q` sale. Un clic en el ícono de
la bandeja muestra u oculta la ventana. No hay atajo de teclado para aprobar.

## Inicio automático

En el panel, la tarjeta **Ajustes** tiene dos interruptores independientes.
Los dos quedan encendidos después de instalar:

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

## Puerto y red

El servidor escucha en `:8765` y se anuncia en la red local por mDNS como
`_nudge._tcp`. Para cambiar el puerto, ejecutá `systemctl --user edit nudge` y
definí `Environment=NUDGE_PORT=...`. Los nombres viejos `CLIPBOARD_SHARE_*`
(por ejemplo `CLIPBOARD_SHARE_PORT`) siguen valiendo como respaldo.

## Permisos de /dev/uinput

Wayland prohíbe la inyección global de input por protocolo, así que el
servidor usa `/dev/uinput`. En Arch el nodo ya es `root:input` con modo
`0660`, de modo que alcanza con que tu usuario esté en el grupo `input`:

```bash
sudo usermod -aG input "$USER"   # y volver a iniciar sesión
```

El log de arranque dice qué modo está activo (`input: uinput injector active`
o el stub de registro).

El paquete instala además una regla con `uaccess`
(`pkgbuild/99-nudge-uinput.rules`) pensada para dar acceso al usuario con
sesión activa sin el grupo. **No está verificada**: en la máquina de
desarrollo el ACL de `/dev/uinput` no mostró ninguna entrada para el usuario y
el acceso venía del grupo `input`.

## Emparejar un teléfono

1. En el teléfono, elegí el PC en la lista de inicio. Se abre el trackpad y,
   si es la primera vez, la app pide el emparejamiento.
2. En el PC aparece una notificación (con el código de verificación) y el
   panel muestra la solicitud.
3. Compará el código: el teléfono muestra uno como `4F2A-91C7-0B3E` y el
   panel (o `nudge pair list`, en su primera línea) tiene que mostrar el
   mismo. Si coinciden, tocá **Aprobar**; si no, **Rechazar**.
4. Listo. Las reconexiones siguientes son automáticas, también después de
   reiniciar el servidor o el PC.

Un teléfono que el PC ya conocía solo se reconecta sin aprobación si
presenta su token vigente por la conexión cifrada; si no lo tiene (por
ejemplo, después del aviso "El certificado de la PC cambió"), la solicitud
vuelve a aparecer en el panel con el código, como la de un teléfono nuevo.
Si un PC con una versión anterior de Nudge igual le da acceso sin pedir
aprobación, es el teléfono el que pregunta "¿La PC muestra el código …?":
miralo en el panel o con `nudge pair list` y tocá **Sí, coincide** solo si
es el mismo. Con **No** el teléfono no guarda nada y cancela el
emparejamiento.

El CLI sirve para lo mismo y es cómodo para scripts:

```bash
nudge pair --last                 # aprueba el request más nuevo
nudge pair list                   # código de verificación, pendientes y aprobados
nudge pair revoke --device <id>   # quitar un dispositivo
```

Atajo opcional para Hyprland (`~/.config/hypr/hyprland.conf`) que aprueba sin
abrir el panel:

```ini
bind = SUPER SHIFT, P, exec, nudge pair --last
```

Si perdés un teléfono, usá **Revocar** en el panel (o
`nudge pair revoke --device <id>`). Para resetear todo el emparejamiento,
borrá `~/.config/nudge/state.json`.

El tráfico con el teléfono va cifrado (HTTPS/WSS) con un certificado
autofirmado que el servidor guarda en `~/.config/nudge/tls.crt` y
`~/.config/nudge/tls.key`. El teléfono recuerda ese certificado al emparejar.
Si borrás `tls.key` (o todo `~/.config/nudge`), el servidor genera otro y
**cada teléfono tiene que emparejarse de nuevo**.

## Migración desde clipboard-share

Nudge antes se llamaba Clipboard Share. El paquete `nudge` reemplaza a
`clipboard-share` (`replaces`/`conflicts`/`provides`). Antes de actualizar,
apagá el servicio viejo:

```bash
systemctl --user disable --now clipboard-share
```

Al primer arranque, el **servidor** mueve `~/.config/clipboard-share` a
`~/.config/nudge` (si este último no existe todavía), así los teléfonos
emparejados siguen funcionando. Un `~/.config/nudge` existente nunca se
sobrescribe. El CLI y el panel no mueven nada: usan `~/.config/nudge` si
existe y, si no, el directorio viejo. Si el servidor no puede moverlo, sigue
trabajando con el directorio viejo y no se pierde ningún emparejamiento.

Durante la transición el servidor anuncia el servicio mDNS nuevo
(`_nudge._tcp`) y el viejo (`_clipboard-share._tcp`), y la app de Android
busca los dos.

## Probar el panel sin el paquete

```bash
panel/bin/build && panel/build/nudge-panel
panel/bin/test    # tests del panel (Qt Test, offscreen)
```
