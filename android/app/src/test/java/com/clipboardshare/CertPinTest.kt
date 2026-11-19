package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CertPinTest {
    // The shared S3 vector: internal/tlscert pins the same pair on the Go side.
    private val vectorFingerprint = "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b"

    @Test
    fun codeOfTheSharedVector() {
        assertEquals("6B86-B273-FF34", CertPin.code(vectorFingerprint))
    }

    @Test
    fun codeAcceptsAnUppercaseFingerprint() {
        assertEquals("6B86-B273-FF34", CertPin.code(vectorFingerprint.uppercase()))
    }

    @Test
    fun codeIsEmptyForInputThatDoesNotStartWith12HexDigits() {
        for (bad in listOf("", "6b86b273ff3", "zz86b273ff34fce1")) {
            assertEquals("code($bad)", "", CertPin.code(bad))
        }
    }

    @Test
    fun fingerprintIsLowercaseSha256Hex() {
        // "1" hashes to the vector fingerprint, as in the Go test.
        assertEquals(vectorFingerprint, CertPin.fingerprint("1".toByteArray()))
    }

    @Test
    fun fingerprintOfACertificateHashesItsSubjectPublicKeyInfo() {
        // Expected value from: openssl x509 -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum
        assertEquals(TestCerts.A_FINGERPRINT, CertPin.fingerprint(TestCerts.a))
        assertEquals(TestCerts.B_FINGERPRINT, CertPin.fingerprint(TestCerts.b))
    }

    @Test
    fun noPinMeansPairing() {
        assertEquals(CertPin.Decision.PAIRING, CertPin.decide(null, vectorFingerprint))
    }

    @Test
    fun matchingPinIsTrusted() {
        assertEquals(CertPin.Decision.TRUSTED, CertPin.decide(vectorFingerprint, vectorFingerprint))
    }

    @Test
    fun differentPinIsAMismatch() {
        assertEquals(
            CertPin.Decision.MISMATCH,
            CertPin.decide(TestCerts.A_FINGERPRINT, TestCerts.B_FINGERPRINT),
        )
    }

    @Test
    fun mismatchStatusAsksToPairAgain() {
        assertEquals("El certificado de la PC cambió. Emparejala de nuevo.", CertPin.MISMATCH_STATUS)
    }

    @Test
    fun waitingTextShowsTheCode() {
        assertEquals(
            "Código: 6B86-B273-FF34. Comprobá que la PC muestre el mismo.",
            CertPin.waitingCodeText(vectorFingerprint),
        )
    }

    @Test
    fun aTokenIsUsableOnlyTogetherWithAPin() {
        assertTrue(CertPin.tokenUsable("abc", vectorFingerprint))
        assertFalse(CertPin.tokenUsable("abc", null))
        assertFalse(CertPin.tokenUsable("abc", ""))
        assertFalse(CertPin.tokenUsable(null, vectorFingerprint))
    }
}
