package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.IOException
import java.security.cert.CertificateException
import javax.net.ssl.SSLHandshakeException

class PinningTrustManagerTest {
    @Test
    fun pinnedTrustManagerAcceptsOnlyTheMatchingCertificate() {
        val tm = PinningTrustManager(TestCerts.A_FINGERPRINT)
        tm.checkServerTrusted(arrayOf(TestCerts.a), "ECDHE_ECDSA")
        assertEquals(TestCerts.A_FINGERPRINT, tm.seenFingerprint)
        assertFalse(tm.mismatchSeen)
    }

    @Test
    fun pinnedTrustManagerRejectsAnotherCertificateAsAMismatch() {
        val tm = PinningTrustManager(TestCerts.A_FINGERPRINT)
        try {
            tm.checkServerTrusted(arrayOf(TestCerts.b), "ECDHE_ECDSA")
            fail("a certificate that does not match the pin was accepted")
        } catch (e: PinMismatchException) {
            assertTrue(PinnedTls.isPinMismatch(e))
        }
        assertTrue(tm.mismatchSeen)
    }

    @Test
    fun pairingTrustManagerAcceptsAndCapturesThePresentedCertificate() {
        val tm = PinningTrustManager(null)
        assertNull(tm.seenFingerprint)
        tm.checkServerTrusted(arrayOf(TestCerts.b), "ECDHE_ECDSA")
        assertEquals(TestCerts.B_FINGERPRINT, tm.seenFingerprint)
        assertFalse(tm.mismatchSeen)
    }

    @Test
    fun emptyChainIsRejectedButIsNotAMismatch() {
        val tm = PinningTrustManager(null)
        try {
            tm.checkServerTrusted(arrayOf(), "ECDHE_ECDSA")
            fail("an empty chain was accepted")
        } catch (e: CertificateException) {
            assertFalse(PinnedTls.isPinMismatch(e))
        }
    }

    @Test
    fun noClientCertificatesAreTrusted() {
        val tm = PinningTrustManager(TestCerts.A_FINGERPRINT)
        assertEquals(0, tm.acceptedIssuers.size)
        try {
            tm.checkClientTrusted(arrayOf(TestCerts.a), "ECDHE_ECDSA")
            fail("client certificates must not be trusted")
        } catch (_: CertificateException) {
        }
    }

    @Test
    fun mismatchIsFoundInsideAWrappedHandshakeFailure() {
        val wrapped = SSLHandshakeException("handshake failed").apply {
            initCause(PinMismatchException(TestCerts.A_FINGERPRINT, TestCerts.B_FINGERPRINT))
        }
        assertTrue(PinnedTls.isPinMismatch(wrapped))
        assertFalse(PinnedTls.isPinMismatch(IOException("connection refused")))
    }

    @Test
    fun clientForAPinUsesThePinningTrustManagerAndSkipsTheHostname() {
        val pinned = PinnedTls.client(okhttp3.OkHttpClient(), TestCerts.A_FINGERPRINT)
        assertEquals(TestCerts.A_FINGERPRINT, pinned.trustManager.pin)
        assertTrue(pinned.http.hostnameVerifier.verify("192.168.0.14", null))
    }
}
