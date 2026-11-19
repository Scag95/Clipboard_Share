Nudge para Windows 10/11 (x64)
==============================

AVISO: esta versión se compiló desde Linux (cross-compile). Se prueba en
GitHub Actions (windows-latest), pero todavía NO se probó en una PC física
con Windows. Si algo falla, mirá el log (ver abajo) y avisá.

Más documentación: https://github.com/Scag95/Nudge/blob/main/docs/windows.md

Nudge comparte el portapapeles (texto e imágenes) entre esta PC y un teléfono
Android en la misma red WiFi, y deja que el teléfono maneje el mouse y el
teclado de la PC, siempre después de aprobarlo en la PC.


Qué hay en el zip
-----------------
  nudge.exe              Programa de consola: el comando "nudge pair ..." y,
                         si lo abrís a mano, el servidor con el log en pantalla.
  nudge-background.exe   El mismo programa sin ventana: el servidor en segundo
                         plano (lo usan el menú Inicio y el inicio automático).
  nudge.ico              Ícono.
  Install Nudge.cmd      Doble clic para instalar.
  Uninstall Nudge.cmd    Doble clic para desinstalar.
  install.ps1, uninstall.ps1, toggle-autostart.ps1, firewall.ps1
                         Los scripts de PowerShell que usan los anteriores.
  README-windows.txt     Este archivo.


Instalar
--------
1. Extraé el zip completo (clic derecho > Extraer todo). No lo ejecutes desde
   dentro del zip.
2. Doble clic en "Install Nudge.cmd". No pide administrador.
   Se instala en %LOCALAPPDATA%\Programs\Nudge y:
   - crea en el menú Inicio la carpeta "Nudge" con: "Nudge" (inicia el
     servidor), "Nudge - autostart on-off" y "Uninstall Nudge";
   - activa el inicio automático al iniciar sesión;
   - agrega la carpeta al PATH del usuario (para "nudge pair" en una
     terminal nueva);
   - pregunta si agregar las reglas del firewall (ver abajo);
   - inicia el servidor.
   Volver a ejecutarlo actualiza la instalación en el lugar.

Opciones (desde PowerShell, en la carpeta extraída):
  powershell -ExecutionPolicy Bypass -File install.ps1 -NoAutostart
  powershell -ExecutionPolicy Bypass -File install.ps1 -FirewallRule yes
  powershell -ExecutionPolicy Bypass -File install.ps1 -FirewallRule no -NoPause


SmartScreen
-----------
Los ejecutables no están firmados. Windows SmartScreen puede mostrar
"Windows protegió su PC": tocá "Más información" > "Ejecutar de todas
formas" solo si descargaste el zip del repositorio oficial y el SHA256
coincide con el archivo .sha256 publicado:
  certutil -hashfile nudge-<versión>-windows-x64.zip SHA256
El instalador quita la marca "descargado de Internet" de los archivos que
copia, para que el inicio automático no avise en cada sesión.


Emparejar el teléfono
---------------------
En la app de Android elegí esta PC. En la PC aparece una ventana
"Sí / No" con el nombre y la IP del teléfono y un código de verificación
(por ejemplo "Código: 4F2A-91C7-0B3E"). El teléfono también muestra un
código: tocá "Sí" solo si reconocés el teléfono y los dos códigos son
iguales (cualquiera en la red puede pedir emparejarse). El botón por
defecto es No: un Enter o un espacio sin querer rechaza. La solicitud
caduca a los 90 s.

El tráfico con el teléfono va cifrado (HTTPS) con un certificado propio de
esta PC (%APPDATA%\nudge\tls.crt y tls.key). Si borrás tls.key o la
carpeta %APPDATA%\nudge, la PC genera otro certificado y cada teléfono
tiene que emparejarse de nuevo.

También se puede desde una terminal (abrila después de instalar):
  nudge pair list      ver el código de verificación, las solicitudes
                       pendientes y los teléfonos aprobados
  nudge pair --last    aprobar la solicitud más nueva
  nudge pair revoke --device <id>   quitar un teléfono


Inicio automático
-----------------
Menú Inicio > Nudge > "Nudge - autostart on-off" activa o desactiva el
inicio al iniciar sesión (un acceso directo en la carpeta Inicio del
usuario). No inicia ni detiene el servidor que ya está corriendo.
Desde PowerShell:
  powershell -ExecutionPolicy Bypass -File "%LOCALAPPDATA%\Programs\Nudge\toggle-autostart.ps1" on
  (on | off | status; sin argumento cambia el estado)


Firewall
--------
El teléfono necesita conectarse a esta PC: TCP 8765 (Nudge) y UDP 5353
(mDNS, para que la encuentre solo). El instalador ofrece crear dos reglas
de entrada, solo para redes PRIVADAS y solo para nudge-background.exe:
"Nudge" y "Nudge mDNS". Hace falta aceptar el aviso de administrador
(UAC); si lo rechazás, la instalación sigue y podés volver a ejecutar el
instalador después.
- Tu red WiFi tiene que estar marcada como "Privada" en Windows.
- Si alguna vez Windows preguntó por Nudge y elegiste "Cancelar", puede
  haber una regla de bloqueo: revisala en "Firewall de Windows Defender >
  Permitir una aplicación".
- Si abrís nudge.exe a mano, Windows puede pedir permiso aparte para ese
  ejecutable.
- Si cambiaste el puerto con NUDGE_PORT, las reglas siguen siendo para 8765.


Desinstalar
-----------
Menú Inicio > Nudge > "Uninstall Nudge" (o "Uninstall Nudge.cmd" del zip).
Detiene Nudge y borra los accesos directos, la entrada del PATH, las
reglas del firewall (pide administrador) y la carpeta de instalación.
Pregunta antes de borrar %APPDATA%\nudge (por defecto No): ahí quedan los
teléfonos emparejados.


Datos y log
-----------
  Configuración y emparejamientos: %APPDATA%\nudge\state.json
  Certificado del servidor:        %APPDATA%\nudge\tls.crt y tls.key
  Log del servidor en segundo plano: %APPDATA%\nudge\nudge.log


Límites conocidos
-----------------
- Probado solo en GitHub Actions (windows-latest), no en una PC física.
- No hay panel como en Linux: el ícono de la bandeja (clic derecho) solo
  muestra el estado y los teléfonos emparejados. La aprobación es con la
  ventana Sí/No o con "nudge pair"; para revocar, "nudge pair revoke".
- Ejecutables sin firmar (SmartScreen).
