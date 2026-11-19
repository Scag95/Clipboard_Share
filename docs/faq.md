# Preguntas frecuentes

Los problemas que más aparecen y cómo salir de ellos. El detalle de cada
plataforma está en [linux.md](linux.md), [windows.md](windows.md) y
[android.md](android.md).

## El teléfono no encuentra la PC

- El celu y la PC tienen que estar en la misma WiFi.
- El servidor tiene que estar corriendo. En Linux:
  `systemctl --user start nudge` (o abrí `nudge-panel`, que lo arranca). En
  la PC misma podés probarlo con `curl -k https://127.0.0.1:8765/health`.
- El celu encuentra la PC por mDNS (`_nudge._tcp`, UDP 5353) y después le
  habla por TCP 8765. Si hay un firewall en el medio, esos dos puertos tienen
  que estar abiertos para la red local. Si cambiaste el puerto con
  `NUDGE_PORT`, es ese en lugar de 8765 ([Puerto y red](linux.md#puerto-y-red)).
- En Windows, la WiFi tiene que estar marcada como **red privada** y tienen
  que existir las reglas "Nudge" (TCP 8765) y "Nudge mDNS" (UDP 5353). Si
  rechazaste el aviso de administrador al instalar, volvé a ejecutar
  `Install Nudge.cmd` y aceptalo ([Firewall](windows.md#firewall)).

## El control remoto no mueve el mouse en Linux

Al servidor le falta acceso a `/dev/uinput`. Agregá tu usuario al grupo
`input` y volvé a iniciar sesión:

```bash
sudo usermod -aG input "$USER"
```

Después mirá el log ([dónde está](#dónde-está-el-log)): tiene que decir
`input: uinput injector active`. La regla `uaccess` del paquete no está
verificada, así que no cuentes con ella sin el grupo
([Permisos de /dev/uinput](linux.md#permisos-de-devuinput)).

Si tenés el paquete `nudge-0.1.0-1`, el control remoto no anda aunque tengas
el permiso: es un error de esa versión. Actualizá a la 1.0.0.

## Android no manda lo que copio con la app cerrada

PC → celu funciona solo. Para celu → PC con la app cerrada hacen falta dos
permisos, una sola vez, con el celu conectado por `adb`:

```bash
adb shell pm grant com.scag95.nudge android.permission.READ_LOGS
adb shell appops set com.scag95.nudge SYSTEM_ALERT_WINDOW allow
```

Además, al abrir la app, aceptá "¿Permitir que Nudge acceda a todos los
registros del dispositivo?" con *Permitir acceso por única vez*. Si el
sistema lo vuelve a pedir (por ejemplo, después de reiniciar el celu),
aceptalo de nuevo. Sin los
permisos, la pantalla remota muestra "Falta permiso READ_LOGS: ver README"
([Sync en segundo plano](android.md#sync-de-clipboard-en-segundo-plano-celu--pc)).

## "El certificado de la PC cambió"

El celu guardó el certificado de la PC al emparejar y ahora ve otro. Pasa
cuando la PC generó uno nuevo, por ejemplo porque se borró `tls.key` o toda
la configuración (`~/.config/nudge`, o `%APPDATA%\nudge` en Windows). El celu
ya borró el token de esa PC: emparejalo de nuevo y aprobalo en la PC
comparando el código.

## Los códigos de verificación no coinciden

Rechazá la solicitud. Si el código del celu no es el que muestra la PC (en el
panel, en la ventana Sí/No o con `nudge pair list`), otro equipo se puede
estar haciendo pasar por tu PC. Antes de reintentar, revisá que elegiste la
PC correcta en la lista ([Cómo se empareja](../README.md#cómo-se-empareja)).

## "La PC usa otra versión de Nudge"

La app y el servidor hablan versiones distintas del protocolo, y la app no se
conecta hasta que coincidan. El mensaje termina diciendo qué actualizar:
"Actualizá Nudge en la PC." o "Actualizá la app.". El token y el pin de
esa PC se conservan, así que después de actualizar no hace falta emparejar
de nuevo ([Versión del protocolo](development.md#versión-del-protocolo)).

## Actualicé a la 1.0 y tengo que emparejar de nuevo

Es normal: la 1.0 es otra app para Android (`com.scag95.nudge`) y cifra el
tráfico, así que un teléfono de la 0.1.0 no sirve. Desinstalá la app vieja
(`adb uninstall com.clipboardshare`), instalá la nueva y emparejá otra vez.
La lista completa de pasos está en
[Para actualizar desde 0.1.0](../CHANGELOG.md#para-actualizar-desde-010).

## Dónde está el log

- Linux: `journalctl --user -u nudge`.
- Windows: `%APPDATA%\nudge\nudge.log`. Si abrís `nudge.exe` a mano, el log
  sale en la consola ([Desinstalar y log](windows.md#desinstalar-y-log)).

## Windows dice "Windows protegió su PC"

Es SmartScreen: los ejecutables no están firmados. Verificá primero el zip
con `certutil -hashfile <zip> SHA256` contra su `.sha256` (o
`SHA256SUMS`), y después elegí *Más información* → *Ejecutar de todas
formas* ([Instalar](windows.md#instalar)).
