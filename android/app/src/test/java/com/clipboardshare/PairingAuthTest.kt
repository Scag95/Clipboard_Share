package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** S3 (#21): /pair/request carries the current token only over a pinned connection. */
class PairingAuthTest {
    private val pin = "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b"

    @Test
    fun storedTokenAndPinSendBearerToken() {
        assertEquals("Bearer abc123", PairingAuth.requestHeaderValue("abc123", pin))
    }

    @Test
    fun noPinMeansNoHeaderEvenWithAToken() {
        assertNull(PairingAuth.requestHeaderValue("abc123", null))
        assertNull(PairingAuth.requestHeaderValue("abc123", ""))
        assertNull(PairingAuth.requestHeaderValue("abc123", "  "))
    }

    @Test
    fun noTokenMeansNoHeaderEvenWithAPin() {
        assertNull(PairingAuth.requestHeaderValue(null, pin))
        assertNull(PairingAuth.requestHeaderValue("", pin))
        assertNull(PairingAuth.requestHeaderValue(" ", pin))
    }

    @Test
    fun neitherTokenNorPinMeansNoHeader() {
        assertNull(PairingAuth.requestHeaderValue(null, null))
    }
}
