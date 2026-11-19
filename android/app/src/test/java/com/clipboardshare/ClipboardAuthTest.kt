package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ClipboardAuthTest {
    @Test
    fun headerNameIsAuthorization() {
        assertEquals("Authorization", ClipboardAuth.HEADER_NAME)
    }

    @Test
    fun headerValueIsBearerWithTheStoredToken() {
        assertEquals("Bearer abc123", ClipboardAuth.headerValue("abc123"))
    }

    @Test
    fun noHeaderForMissingToken() {
        assertNull(ClipboardAuth.headerValue(null))
    }

    @Test
    fun noHeaderForEmptyOrBlankToken() {
        assertNull(ClipboardAuth.headerValue(""))
        assertNull(ClipboardAuth.headerValue("   "))
    }

    @Test
    fun syncRunsOnlyWithAToken() {
        assertTrue(ClipboardAuth.canSync("abc123"))
        assertFalse(ClipboardAuth.canSync(null))
        assertFalse(ClipboardAuth.canSync(""))
        assertFalse(ClipboardAuth.canSync(" "))
    }

    @Test
    fun unauthorizedRequiresPairingAgain() {
        assertEquals(ClipboardAuth.Outcome.REPAIR, ClipboardAuth.classify(401))
    }

    @Test
    fun successCodesAreOk() {
        assertEquals(ClipboardAuth.Outcome.OK, ClipboardAuth.classify(200))
        assertEquals(ClipboardAuth.Outcome.OK, ClipboardAuth.classify(204))
        assertEquals(ClipboardAuth.Outcome.OK, ClipboardAuth.classify(299))
    }

    @Test
    fun otherCodesAreTransientErrors() {
        for (code in listOf(199, 300, 400, 403, 404, 413, 500, 503)) {
            assertEquals("HTTP $code", ClipboardAuth.Outcome.TRANSIENT_ERROR, ClipboardAuth.classify(code))
        }
    }

    @Test
    fun repairStatusAsksToPairAgain() {
        assertEquals("La PC rechazó este teléfono. Emparejalo de nuevo.", ClipboardAuth.REPAIR_STATUS)
    }
}
