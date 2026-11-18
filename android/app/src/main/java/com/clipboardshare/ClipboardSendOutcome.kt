package com.clipboardshare

/**
 * Result of one manual phone -> PC send (Quick Settings tile or notification action).
 * Pure Kotlin so the user-facing texts can be unit-tested without Android.
 */
enum class ClipboardSendOutcome {
    /** The PC accepted the clipboard content. */
    SENT,

    /** The clipboard was empty or could not be read. */
    EMPTY,

    /** The clipboard holds an image and image sharing is turned off. */
    IMAGE_NOT_SHARED,

    /** The PC answered but did not accept the content. */
    REJECTED,

    /** The PC could not be reached. */
    UNREACHABLE,

    /** No sync service is running, so there is no PC to send to. */
    NO_SYNC,
    ;

    /** Status text shown on the remote screen and in the toast; [host] is "ip:port". */
    fun statusText(host: String): String = when (this) {
        SENT -> "Portapapeles enviado a $host"
        EMPTY -> "Portapapeles vacío: no hay texto para enviar"
        IMAGE_NOT_SHARED -> "Imagen local no compartida"
        REJECTED -> "Error al enviar el portapapeles"
        UNREACHABLE -> "Error al enviar el portapapeles: sin conexión con $host"
        NO_SYNC -> "Sincronización inactiva: elegí un host en Clipboard Share"
    }
}
