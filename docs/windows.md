# Nudge en Windows (beta)

Hay un build para Windows 10/11 x64. Se compila desde Linux y el workflow
[`windows.yml`](../.github/workflows/windows.yml) lo prueba en GitHub Actions
sobre `windows-latest`: corre los tests de Go y prueba el zip de punta a punta
(instalador, servidor, CLI por el socket de control, emparejamiento,
clipboard y desinstalador). **Todavía no lo probé en una PC física con
Windows**, así que puede fallar. Si algo no anda, mirá el
[log](#desinstalar-y-log) y avisá.

## Qué trae el zip

Se descarga como `nudge-<versión>-windows-x64.zip` (con su `.sha256`) desde la
[página de releases](https://github.com/Scag95/Nudge/releases). Trae dos
ejecutables del mismo código:

- `nudge.exe`: consola. Es el CLI `nudge pair` y, abierto a mano, el servidor
  con el log en pantalla.
- `nudge-background.exe`: sin ventana. Es el servidor en segundo plano.

Además vienen el ícono, los scripts de instalación y `README-windows.txt`.

## Instalar

1. Extraé el zip completo (clic derecho → *Extraer todo*).
2. Doble clic en `Install Nudge.cmd`. No pide administrador: instala en
   `%LOCALAPPDATA%\Programs\Nudge`, crea la carpeta **Nudge** en el menú
   Inicio ("Nudge", "Nudge - autostart on-off", "Uninstall Nudge"), activa el
   inicio automático, agrega la carpeta al `PATH` del usuario e inicia el
   servidor. Volver a ejecutarlo actualiza en el lugar.

Opciones de `install.ps1`: `-NoAutostart`, `-FirewallRule yes|no` (para
instalar sin preguntas), `-NoStart`, `-NoPause`.

Los ejecutables **no están firmados**: SmartScreen puede mostrar "Windows
protegió su PC" (*Más información* → *Ejecutar de todas formas*). Verificá
antes el SHA256 (`certutil -hashfile <zip> SHA256`).

## Emparejar

Cuando un teléfono pide emparejarse, el ícono de la bandeja muestra una
notificación ("Solicitud de emparejamiento") y aparece una ventana **Sí/No**
con su nombre, su IP y el código de verificación de la PC
(`Código: XXXX-XXXX-XXXX`), con **No** como botón por defecto (un Enter sin
querer rechaza). Elegí **Sí** solo si el teléfono muestra el mismo código.
La ventana es el único lugar donde se aprueba. El CLI también sirve, desde
una terminal nueva: `nudge pair list` (la primera línea es
`Verification code: ...`) y `nudge pair --last`.

El tráfico con el teléfono va cifrado (HTTPS/WSS) con un certificado
autofirmado que el servidor guarda en `%APPDATA%\nudge\tls.crt` y
`%APPDATA%\nudge\tls.key`; el teléfono lo recuerda al emparejar. Si borrás
`tls.key` (o `%APPDATA%\nudge`, por ejemplo al desinstalar con borrado de
datos), el servidor genera otro y **cada teléfono tiene que emparejarse de
nuevo**.

## Ícono en la bandeja

Mientras el servidor corre, Nudge tiene un ícono en el área de notificación.
Con clic derecho muestra el estado ("Escuchando en el puerto 8765") y los
teléfonos emparejados con su IP ("Ningún teléfono emparejado" si no hay).
Es solo informativo: para revocar un teléfono usá
`nudge pair revoke --device <id>` (el id sale en `nudge pair list`). Si
Windows esconde el ícono, está en la flechita "Mostrar iconos ocultos". Si
el explorador se reinicia, el ícono vuelve solo.

## Inicio automático

"Nudge - autostart on-off" en el menú Inicio activa o desactiva el acceso
directo de la carpeta Inicio del usuario (también
`toggle-autostart.ps1 on|off|status`). No inicia ni detiene el servidor que ya
corre.

## Firewall

El instalador explica y pregunta si agregar dos reglas de entrada, solo en
redes **privadas** y solo para `nudge-background.exe`: "Nudge" (TCP 8765) y
"Nudge mDNS" (UDP 5353). Hace falta aceptar el aviso de administrador (UAC);
si lo rechazás, la instalación sigue y podés volver a ejecutarla después. La
WiFi tiene que estar marcada como red privada.

## Desinstalar y log

Menú Inicio → Nudge → "Uninstall Nudge" (o `Uninstall Nudge.cmd` del zip).
Detiene el servidor y borra los accesos directos, la entrada del `PATH`, las
reglas del firewall y la carpeta de instalación. Pregunta antes de borrar
`%APPDATA%\nudge` (por defecto No), donde quedan los teléfonos emparejados
(`state.json`).

El servidor en segundo plano escribe su log en `%APPDATA%\nudge\nudge.log`.

## Límites

- Probado solo en GitHub Actions (`windows-latest`), no en una PC física.
- No hay panel como en Linux: el ícono de la bandeja solo informa; aprobar es
  con la ventana Sí/No y revocar con `nudge pair revoke`.
- Ejecutables sin firmar (SmartScreen).

## Recompilar el zip

Desde Linux, con Go 1.24, `rsvg-convert`, ImageMagick (`magick`) y `zip`:

```bash
scripts/build-windows.sh 0.1.0
# salida: dist/nudge-0.1.0-windows-x64.zip y su .sha256
```

Para compilar solo el `.exe`, ver
[Compilar los binarios](development.md#compilar-los-binarios).
