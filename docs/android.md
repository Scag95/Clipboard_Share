# App de Android (beta personal)

La app descubre el PC en la red, sincroniza el clipboard en los dos sentidos y
hace de trackpad y teclado remoto. Es una beta de uso personal: APK con firma
debug, instalada a mano, sin Play Store.

## Instalar

Hay un APK ya compilado (`nudge-<versión>-debug.apk`) en la
[página de releases](https://github.com/Scag95/Nudge/releases). También podés
instalarlo por USB con `adb`, o con `adb connect` por Wi-Fi.

## Generar el APK

Requiere JDK 17 y Android SDK (platform 34 + build-tools 34). `android/` trae
el wrapper de Gradle, así que no hace falta tener Gradle instalado:

```bash
cd android
./gradlew assembleDebug
# salida: app/build/outputs/apk/debug/app-debug.apk (firma debug)
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

Para trabajar en la app con Android Studio:

- Android Studio (Hedgehog o superior).
- JDK 17.
- Android SDK 24+ (minSdk).
- Gradle 8+.
- Dependencias previstas: Kotlin stdlib; AndroidX (AppCompat, Core KTX);
  OkHttp / Retrofit (cliente HTTP); `javax.jmdns` o NSD nativo
  (`NsdManager`) para el descubrimiento mDNS; Coroutines para las
  operaciones asíncronas.

## Uso

Al abrir la app aparece la lista de PCs (los que están en la red y los que ya
conocés). Elegí uno para abrir el trackpad; la primera vez la app pide el
emparejamiento y hay que aprobarlo en el PC (ver [Linux](linux.md#emparejar-un-teléfono)
o [Windows](windows.md#emparejar)). El control remoto requiere la pantalla
abierta.

## Sync de clipboard en segundo plano (celu → PC)

Al elegir un host arranca un servicio con notificación persistente (requisito
de Android) que mantiene **PC → celu** en segundo plano sin configurar nada.

Para el sentido contrario, **celu → PC con la app cerrada**, hay que darle dos
permisos que Android no deja pedir desde la interfaz. Se hace una vez, con el
celu conectado por `adb`:

```bash
adb shell pm grant com.clipboardshare android.permission.READ_LOGS
adb shell appops set com.clipboardshare SYSTEM_ALERT_WINDOW allow
```

Además Android muestra, al abrir la app, el diálogo **"¿Permitir que Nudge
acceda a todos los registros del dispositivo?"**: elegí *Permitir acceso por
única vez*. Es un consentimiento tuyo sobre datos sensibles y solo puede
aceptarse con la app abierta. Si el sistema lo vuelve a pedir (reinicio del
celu o del proceso), aceptalo de nuevo.

### Cómo funciona

Android niega el clipboard a las apps sin foco, pero registra en el log del
sistema cada cambio dirigido a una app sin foco. El servicio lee esa línea
(solo la de su propio paquete), abre una fracción de segundo una ventana
transparente que gana foco, lee el clipboard, lo envía a la PC y la cierra.
Mientras tanto retiene el clipboard de la PC para que tu copia gane.

Los logs de la app nunca incluyen el contenido copiado: solo el tipo y el
tamaño (`<text 9 chars>`).

### Mensajes de estado

- Sin los permisos, la pantalla remota muestra "Falta permiso READ_LOGS: ver
  README" y el servicio sigue sincronizando solo PC → celu. Los comandos
  están [más arriba](#sync-de-clipboard-en-segundo-plano-celu--pc).
- Si los permisos están pero pasan 60 s sin que llegue ninguna línea del log,
  el estado agrega "Sin datos del log del sistema". Suele ser el diálogo de
  acceso a registros sin aceptar (también puede aparecer si simplemente no
  hubo copias en ese tiempo).

### Por qué es frágil

Depende del texto de un log del sistema, que Android puede cambiar entre
versiones. Está probado en Android 16 (Samsung S24), con el servicio del
paquete corriendo en el PC.

Una escritura propia en segundo plano genera **dos** líneas de log (~340 ms
entre sí), y el servicio las trata como eco propio para no abrir la ventana de
lectura tras cada copia que llega de la PC.

## Tests

```bash
cd android && ./gradlew testDebugUnitTest
```

JUnit sobre la lógica de disparo del sync en segundo plano.
