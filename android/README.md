# Android client

Cliente Android para Clipboard Share: sincroniza el clipboard (texto e
imágenes PNG) con el PC y ofrece **control remoto de mouse y teclado**.

Stack: Kotlin + AndroidX + OkHttp + NSD (Network Service Discovery) +
WebSocket para eventos de input.

## Pantallas

- **MainActivity** — descubrimiento del server por NSD
  (`_clipboard-share._tcp`) y sync bidireccional del clipboard (polling 2 s +
  listener local). Botón de acceso a Control remoto.
- **RemoteControlActivity** ("Control remoto") — touchpad relativo:
  - un dedo = mover puntero; toque corto = click izquierdo;
  - mantener presionado o toque de dos dedos = click derecho;
  - arrastrar con dos dedos = scroll;
  - botones de shortcuts (Ctrl+C/V/X/Z, Esc, Enter, flechas) y barra para
    enviar texto al PC;
  - **Soltar** corta la sesión de input al instante.

## Pairing (emparejamiento)

1. El server imprime en consola un **Pairing PIN** de 6 dígitos al arrancar.
2. En Control remoto se ingresa el PIN y se toca **Emparejar**: se obtiene un
   token de sesión que queda guardado en la app.
3. La conexión WebSocket (`/ws/input?token=...`) abre el stream de eventos.
   Si el server rechaza el token (PIN rotado por intentos fallidos, server
   reiniciado), la app vuelve a pedir el PIN.

Tras 5 PINs incorrectos seguidos el server rota el PIN e imprime uno nuevo.

## Build

Abrir `android/` en Android Studio (o `./gradlew assembleDebug` cuando exista
el wrapper). Requiere JDK 17 y SDK 34 (minSdk 24).

## Estado

Implementado: sync de clipboard, pairing por PIN, touchpad + teclado remoto.
Sin tests instrumentados todavía; la build no se verificó en este entorno
(herramientas de Android no disponibles) — véase `odd/tasks/input-control.md`.
