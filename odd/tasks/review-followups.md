# Feature: review-followups — arreglar los hallazgos no bloqueantes de los reviews

Cierra los hallazgos informativos que dejaron los dos reviews nativos (BG2 en
Android, `review-42138c68e1bdbbdb`, y el panel de Linux,
`review-09f71545fcec346f`), más el alta del panel en el launcher.

## Specs

- **S1** — Verbatim: "arregla todos los errores, te deje el celular conectado y
  desbloqueado. si ya compilaste la app, añadelo al inicio para poder buscarlo
  en la barra de aplicaicones."

- **S2 — Panel (`panel/`)**: arreglar los 13 hallazgos del review del panel.
  Cada uno con su prueba cuando es comportamiento (test primero, observar RED):
  - **P1** (R1-003) El socket de activación de instancia única usa un nombre
    pelado (archivo compartido en el directorio temporal); ubicarlo en el
    directorio de runtime del usuario, como el lock.
  - **P2** (R2-001) `itemWithText(QQuickWindow*, ...)` en `ui_tests.cpp`: su
    comentario habla de un overlay que el código no busca y tiene un
    `if found return found; return nullptr` redundante. Dejar el helper honesto
    (si el overlay es alcanzable desde el content item, decirlo; si no,
    buscarlo).
  - **P3** (R2-003) `waitChanged(spy, count)` en `panel_tests.cpp` no espera
    `count` señales: hacerlo correcto o quitar el parámetro.
  - **P4** (R2-004) El acento por defecto `#FFD60A` está escrito en
    `readAccent` y en `theme.h`: una constante con nombre y comentario.
  - **P5** (R2-005) La extracción del error (incluido el texto por defecto
    "The server rejected the command") está duplicada en `backend.cpp`
    junto a `m_errorFromAction`: un helper.
  - **P6** (R3-001) Los `Repeater` se enlazan a `backend.requests/devices` y
    `Backend` emite `changed()` en cada poll de 1 s con una lista nueva aunque
    no cambió nada: los delegates se recrean cada segundo y un clic que cruza
    un refresco puede perderse. Emitir `changed()` solo si el estado cambió
    de verdad (comparar), con test que lo pruebe.
  - **P7** (R3-002 y R4-single-instance-break) Carrera de instancia única: la
    primera instancia toma el lock antes de cargar QML y escucha en el socket
    de activación recién después; una segunda que no logra conectar en 500 ms
    borra el lock a la fuerza y arranca un segundo panel aunque el primero
    siga vivo (dos íconos de bandeja, dos pollers). Además `activation.listen`
    nunca se comprueba. Arreglo: escuchar la activación antes de cargar QML,
    comprobar `listen`, y no forzar el borrado del lock si su dueño sigue
    vivo (`QLockFile::getLockInfo`).
  - **P8** (R3-003) `setOffline()` vacía `m_seenRequests`: un fallo transitorio
    hace que, al volver, se vuelvan a anunciar todas las solicitudes
    pendientes. No olvidar las solicitudes vistas por un corte; test.
  - **P9** (R3-004) El refresco pedido tras una acción se omite si hay un
    `list` en vuelo, y el resultado viejo puede pisar la acción. Si hay uno en
    vuelo, encolar un refresco para cuando termine; test.
  - **P10** (R3-005) El camino de timeout (servidor que acepta y nunca
    responde) no tiene test: añadir un modo "servidor mudo" al fake y probar
    que el panel pasa a offline y sigue pollenado.
  - **P11** (R3-006) `serverErrorIsShownAndKeepsTheListAndTheServerOnline` no
    prueba que el error de acción sobrevive al `list` posterior: esperar el
    `list` siguiente y reafirmar.

- **S3 — Android**: arreglar los hallazgos del review de BG2 en
  `ClipboardSyncService.kt` / `ClipboardReadActivity.kt`:
  - **A1** (R2-001) El callback de gracia de 5 s que limpia
    `readActivityInFlight` no se guarda ni se cancela: uno viejo puede limpiar
    la bandera de una lectura posterior. Guardar el `Runnable` y cancelarlo en
    `readActivityFinished`.
  - **A2** (R2-002) Constantes de tiempo con dependencia de orden sin
    documentar (`READ_ACTIVITY_GRACE_MS` 5 s, `OWN_CHANGE_QUIET_MS` 1.5 s,
    `TRIGGER_DEBOUNCE_MS` 0.4 s, `LOCAL_READ_PENDING_MS` 3 s, el timeout de la
    activity): comentar la relación entre ellas.
  - **A3** (R2-003) El marcador frágil del log de sistema ("Denying clipboard
    access to ...") es un literal enterrado: constante con nombre que
    documente su origen.
  - **A4** (R3-002) El watcher de logs mira `READ_LOGS` una sola vez al
    iniciar y sale para siempre: si se concede con el servicio ya corriendo,
    nunca arranca y el estado dice "todo listo". Re-chequear periódicamente.
  - **A5** (R3-003) Cada aplicación remota suprime 1.5 s los disparadores del
    log: una copia real del usuario en esa ventana se descarta y nunca se
    envía. No perder la copia (enviar al terminar la ventana si hubo un
    disparador suprimido).
  - **A6** (R3-004 y R4-003) Carrera entre `stopLogWatcher` y el hilo
    watcher: si `stop` corre entre `ProcessBuilder.start()` y asignar
    `logWatcher`, el `logcat` nuevo nunca se destruye y el hilo sigue
    llamando a `onClipboardChangeDetected` en un servicio destruido.
    Sincronizar la asignación y el `destroy`, y comprobar si ya se pidió
    parar tras `start()`.
  - **A7** (R3-005 y R4-005) `syncLocalNow` lanza un `runSync` que puede
    correr a la vez que el del bucle de polling; ambos mutan `lastSyncedItem`
    y el estado sin exclusión: el PC puede pisar la copia del celu o subirse el
    mismo ítem dos veces. Serializar con un `Mutex`.
  - **A8** (R4-002) `logcat -T 1` repite la última línea coincidente en cada
    (re)inicio del watcher, y si era un viejo "Denying clipboard access"
    dispara la lectura sin copia nueva. Empezar desde "ahora" o ignorar las
    líneas anteriores al arranque.
  - **A9** (R4-004) Sin el consentimiento único de acceso a logs de Android
    12+, el watcher no recibe nada y la UI dice "sincronizado" sin avisar:
    registrar si llegó alguna línea desde el inicio y mostrar un aviso si no.
  - **A10** (R3-006) La lógica de disparadores (supresión de cambios propios,
    debounce, guardia en vuelo y su gracia, retención del remoto mientras hay
    una lectura local pendiente) no tiene tests: extraerla a una clase pura
    sin dependencias de Android y probarla con JUnit. (R2-004 ya lo resolvió
    `describeForLog`.)

- **S4 — Launcher**: instalación de **usuario**, sin sudo: binarios en
  `~/.local/bin`, `clipboard-share.desktop` en `~/.local/share/applications`,
  ícono en `~/.local/share/icons/hicolor/scalable/apps`, y refrescar la base
  de datos de desktop files, para que "Clipboard Share" salga al buscar en el
  launcher (`Super + Space`).

## Tasks

- [ ] **T1** (S2) — Panel P1-P11. Route: worker A (`panel/**`), test-first. Commit: pendiente
- [ ] **T2** (S3) — Android A1-A10. Route: worker B (`android/**`). Commit: pendiente
- [ ] **T3** (S4) — Alta en el launcher. Route: inline. Commit: n/a (instalación local)
- [ ] **T4** (S3) — Probar en el celu (R1 sin contenido + camino feliz). Route: inline. Commit: n/a

## Riesgos

- Risk (T2): item 4 (concurrencia: A6, A7) → commit propio y verificador
  independiente.
- Risk (T1): item 4 acotado (P7 carrera de instancia única, P9 cola de
  refresco) → commit propio.

## Log

- **L1** — "arregla todos los errores, te deje el celular conectado y
  desbloqueado. si ya compilaste la app, añadelo al inicio para poder buscarlo
  en la barra de aplicaicones."
