package com.clipboardshare

import java.security.MessageDigest
import java.security.cert.X509Certificate

/**
 * Certificate fingerprint, verification code and pin decision (S3/S6). The Go server
 * (internal/tlscert) computes the same fingerprint and code; both suites pin one vector.
 */
object CertPin {
    const val MISMATCH_STATUS = "El certificado de la PC cambió. Emparejala de nuevo."

    /** How a connection treats the certificate it was shown. */
    enum class Decision {
        /** No pin stored yet: trust on first use, protected by the code comparison. */
        PAIRING,

        /** The certificate is the pinned one. */
        TRUSTED,

        /** A pin is stored and this certificate is another one: refuse it. */
        MISMATCH,
    }

    /** Lowercase hex SHA-256 of a SubjectPublicKeyInfo DER. */
    fun fingerprint(spkiDer: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(spkiDer)
            .joinToString("") { "%02x".format(it.toInt() and 0xff) }

    /** Fingerprint of a certificate: `publicKey.encoded` is its SubjectPublicKeyInfo DER. */
    fun fingerprint(cert: X509Certificate): String = fingerprint(cert.publicKey.encoded)

    /**
     * The first 12 hex digits of [fingerprintHex], uppercase, in three groups of 4 joined
     * by "-"; "" when the input does not start with 12 hex digits.
     */
    fun code(fingerprintHex: String): String {
        if (fingerprintHex.length < 12) return ""
        val head = fingerprintHex.substring(0, 12).uppercase()
        if (!head.all { it in '0'..'9' || it in 'A'..'F' }) return ""
        return head.substring(0, 4) + "-" + head.substring(4, 8) + "-" + head.substring(8, 12)
    }

    fun decide(pin: String?, seen: String): Decision = when {
        pin == null -> Decision.PAIRING
        pin.equals(seen, ignoreCase = true) -> Decision.TRUSTED
        else -> Decision.MISMATCH
    }

    /** Shown under the waiting text so the user compares it with the code on the PC. */
    fun waitingCodeText(fingerprintHex: String): String =
        "Código: ${code(fingerprintHex)}. Comprobá que la PC muestre el mismo."

    /** A token is only used together with the pin of the PC that issued it. */
    fun tokenUsable(token: String?, pin: String?): Boolean =
        !token.isNullOrEmpty() && !pin.isNullOrEmpty()
}
