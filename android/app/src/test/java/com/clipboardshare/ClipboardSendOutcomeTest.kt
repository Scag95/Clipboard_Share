package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Test

class ClipboardSendOutcomeTest {

    private val host = "192.168.1.10:8080"

    @Test
    fun sentNamesTheHost() {
        assertEquals(
            "Portapapeles enviado a $host",
            ClipboardSendOutcome.SENT.statusText(host),
        )
    }

    @Test
    fun emptyClipboardIsReportedInsteadOfFailingSilently() {
        assertEquals(
            "Portapapeles vacío: no hay texto para enviar",
            ClipboardSendOutcome.EMPTY.statusText(host),
        )
    }

    @Test
    fun imageWithImageSharingOffIsNotSent() {
        assertEquals(
            "Imagen local no compartida",
            ClipboardSendOutcome.IMAGE_NOT_SHARED.statusText(host),
        )
    }

    @Test
    fun serverRejectionIsASendError() {
        assertEquals(
            "Error al enviar el portapapeles",
            ClipboardSendOutcome.REJECTED.statusText(host),
        )
    }

    @Test
    fun unreachableHostIsASendErrorNamingTheHost() {
        assertEquals(
            "Error al enviar el portapapeles: sin conexión con $host",
            ClipboardSendOutcome.UNREACHABLE.statusText(host),
        )
    }

    @Test
    fun noRunningSyncAsksToPickAHost() {
        assertEquals(
            "Sincronización inactiva: elegí un host en Clipboard Share",
            ClipboardSendOutcome.NO_SYNC.statusText(host),
        )
    }

    @Test
    fun statusTextsNeverMentionTheRemovedLogTrigger() {
        for (outcome in ClipboardSendOutcome.values()) {
            val text = outcome.statusText(host)
            assertEquals(false, text.contains("READ_LOGS"))
            assertEquals(false, text.contains("registros"))
            assertEquals(false, text.contains("log del sistema"))
        }
    }
}
