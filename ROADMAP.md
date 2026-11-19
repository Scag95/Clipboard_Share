# Hoja de ruta hacia la 1.0

Estado al 19 de noviembre de 2026. El seguimiento vive en el milestone
[1.0](https://github.com/Scag95/Nudge/milestone/1) de GitHub; este archivo
resume lo que falta y en qué orden.

## Hecho (en PR encadenados, sin mergear)

| PR | Rama | Qué trae |
|----|------|----------|
| #9 | `feat/windows-polish` | Bandeja e ícono en Windows, fix de uinput en Linux, README nuevo, limpieza del repo |
| #19 | `feat/clipboard-auth` | Token obligatorio en el portapapeles (#10) |
| #20 | `feat/android-release` | App `com.scag95.nudge` 1.0.0 y firma de release (#14) |
| #22 | `feat/tls` | HTTPS/WSS, pinning y código de verificación (#11) |
| #23 | `feat/repair-auth` | Un teléfono conocido solo renueva su token presentando el actual (#21) |
| #24 | `feat/v1-cleanup` | Sin compatibilidad con Clipboard Share (#13), `go test` seguro (#16) |
| #25 | `feat/protocol-version` | Versión del protocolo (#12) |
| — | `feat/release` | Workflow de release (#15), CHANGELOG y FAQ (#17), esta hoja de ruta |

Cada PR sale del anterior: se mergean en ese orden y, al mergear uno, el
siguiente se cambia a base `main`.

## Próximos pasos

1. **Abrir el PR de `feat/release`** sobre `feat/protocol-version` (#25). Ese
   PR corre el workflow de release en modo prueba: compila el paquete de Arch,
   el zip de Windows y el APK sin firmar, sin publicar nada. Es lo único del
   workflow que todavía no corrió en GitHub.
2. **Cargar los secretos de firma** (una sola vez; detalle en
   [docs/development.md](docs/development.md#publicar-una-versión)):
   ```bash
   base64 -w0 ~/keys/nudge-release.jks | gh secret set NUDGE_KEYSTORE_B64 -R Scag95/Nudge
   gh secret set NUDGE_KEYSTORE_PASSWORD -R Scag95/Nudge
   gh secret set NUDGE_KEY_PASSWORD -R Scag95/Nudge   # solo si la clave tiene otra contraseña
   ```
   Opcional: correr el workflow a mano con `dry_run` en `false` para comprobar
   que el APK sale firmado con tu clave, sin publicar.
3. **Mergear la cadena** en orden: #9, #19, #20, #22, #23, #24, #25 y el de
   `feat/release`. Con todo en `main`, el workflow `windows` vuelve a correr
   ahí y el badge del README deja de decir "sin estado".
4. **Ronda de pruebas reales (#18).** Lista mínima:
   - Windows en una PC física: instalar con `Install Nudge.cmd`, SmartScreen,
     regla del firewall, ícono y menú de la bandeja, globo de la notificación,
     diálogo de emparejamiento con el código, portapapeles en los dos sentidos,
     mouse y teclado, desinstalar.
   - Teléfono: desinstalar la app 0.1.0, instalar el APK firmado, emparejar
     comparando el código en los dos lados, portapapeles (incluido el sync en
     segundo plano), control remoto, y el aviso "El certificado de la PC
     cambió" al borrar `tls.key` en la PC.
   - Otro Android además del Samsung S24, si hay uno a mano.
   - Linux: paquete 1.0.0, panel con el código, regla udev `uaccess` sin
     agregar el usuario a ningún grupo.
5. **Publicar `v1.0.0-rc1`:**
   - Versiones en `1.0.0-rc1`: `cmd/server/version.go`, `versionName` en
     `android/app/build.gradle.kts` (y subir `versionCode`), y en
     `pkgbuild/PKGBUILD` `pkgver=1.0.0_rc1` (pacman no acepta `-`).
   - En `CHANGELOG.md`, una sección `## [1.0.0-rc1]`: el workflow toma las
     notas de la sección con el nombre exacto del tag.
   - `git tag v1.0.0-rc1 && git push origin v1.0.0-rc1`: el workflow publica
     la pre-release.
6. **Publicar `v1.0.0`** si la rc no muestra nada bloqueante: versiones en
   `1.0.0`, fecha en `## [1.0.0]` del CHANGELOG, actualizar la fila de
   Android del README (hoy dice "APK con firma debug") y pushear el tag
   `v1.0.0`.

## Pendientes menores (no bloquean la 1.0)

Observaciones de las revisiones que quedaron como mejoras:

- **Tests que faltan:** el ciclo de vida de la bandeja de Windows, el
  cableado de `haltSync` y del diálogo de confirmación en Android, y la
  normalización `version4` del script de Windows.
- **Bandeja de Windows:** si al volver a agregar el ícono falla `NIM_ADD`
  justo después de borrarlo (cambio de DPI), el ícono no vuelve hasta el
  próximo `TaskbarCreated`. El servidor sigue andando.
- **Android:**
  - un token guardado en blanco se trata como token nuevo en el 401 (usar la
    misma regla que `canSync`);
  - una copia en el primer instante del sync puede llegar una vez a una PC
    incompatible antes del chequeo de protocolo.
- **CI:** fijar las actions por SHA en vez de por versión, y un grupo
  `concurrency` en `windows.yml`.
- **Servidor:** `MkdirAll(dir, 0700)` no ajusta los permisos de un
  directorio de config que ya existía (la clave igual queda en 0600).
- **Opcional:** renombrar el paquete Kotlin interno `com.clipboardshare` (no
  afecta a nadie; el id de la app ya es `com.scag95.nudge`).
