# KDE-style phone-to-PC clipboard send
Replace the logcat/READ_LOGS background clipboard trigger with a manual send, like KDE Connect, so no adb-only permission and no "access all device logs" dialog are needed.
Branch: feat/input-control. Base commit: 0c214de.

## Specs
- S1: The user chose: "haz lo mismo que KDE. sin ADB." Phone to PC clipboard sending is manual, as in KDE Connect: a "Enviar portapapeles" action on the sync service's ongoing notification and a Quick Settings tile. One tap sends the phone's current clipboard text to the PC.
- S2: No permission that needs adb: remove READ_LOGS and SYSTEM_ALERT_WINDOW from the manifest, and remove the logcat watcher and the transparent-window trigger they enabled. Opening the app must not show the system dialog "¿Permitir que Clipboard Share acceda a todos los registros del dispositivo?".
- S3: PC to phone clipboard sync keeps working unchanged, automatically.
- S4: The clipboard status text no longer mentions READ_LOGS ("Falta permiso READ_LOGS: ver README") or the missing system log data; the README section about READ_LOGS/adb grants is replaced by the manual send behaviour.
- S5: Reading the clipboard happens only while the app has focus (a transparent focused activity opened by the tap on the tile or notification action is fine); if the read yields nothing, say so in the status instead of failing silently.
- S6: `./gradlew --offline :app:testDebugUnitTest :app:assembleDebug` passes (use JAVA_HOME=~/.local/share/mise/installs/java/temurin-17.0.21+9). Tests that only covered the removed logcat trigger go away with it; any new pure logic gets a unit test.

## Tasks
- T1 (S1-S6) route: worker, one writer. Commit: pending (the parent commits).

## Log
- L1: haz lo mismo que KDE. sin ADB.
