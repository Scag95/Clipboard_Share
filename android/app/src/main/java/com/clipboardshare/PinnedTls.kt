package com.clipboardshare

import okhttp3.OkHttpClient
import java.security.SecureRandom
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager

/** The PC presented a certificate whose fingerprint is not the stored pin. */
class PinMismatchException(pin: String, seen: String) :
    CertificateException("certificate $seen does not match pin $pin")

/**
 * Trusts the PC's self-signed certificate by its S3 fingerprint, without any CA (S6).
 * With a [pin] only a leaf certificate with that fingerprint is accepted. Without one
 * (pairing) any certificate is accepted and its fingerprint is captured, so the caller
 * can show the code and pin exactly that certificate for the following calls.
 */
class PinningTrustManager(val pin: String?) : X509TrustManager {
    /** Fingerprint of the last certificate checked by this trust manager. */
    @Volatile
    var seenFingerprint: String? = null
        private set

    /** Set once a certificate was refused because it did not match [pin]. */
    @Volatile
    var mismatchSeen: Boolean = false
        private set

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val leaf = chain?.firstOrNull() ?: throw CertificateException("empty certificate chain")
        val seen = CertPin.fingerprint(leaf)
        seenFingerprint = seen
        when (CertPin.decide(pin, seen)) {
            CertPin.Decision.TRUSTED, CertPin.Decision.PAIRING -> Unit
            CertPin.Decision.MISMATCH -> {
                mismatchSeen = true
                throw PinMismatchException(pin ?: "", seen)
            }
        }
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        throw CertificateException("client certificates are not trusted")
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
}

/** An OkHttp client bound to one pin (or to pairing mode when the pin is null). */
class PinnedClient(val http: OkHttpClient, val trustManager: PinningTrustManager) {
    /** Whether [t] failed because the PC presented a certificate other than the pin. */
    fun isMismatch(t: Throwable): Boolean = trustManager.mismatchSeen || PinnedTls.isPinMismatch(t)
}

object PinnedTls {
    /** The pin identifies the PC; its address changes with DHCP and is not in the certificate. */
    private val anyHostname = HostnameVerifier { _, _ -> true }

    /**
     * A client derived from [base] (same pool, dispatcher and timeouts) that trusts only
     * [pin], or captures the certificate in pairing mode when [pin] is null. Each call has
     * its own SSLContext, so its TLS sessions are never resumed by another pin.
     */
    fun client(base: OkHttpClient, pin: String?): PinnedClient {
        val trustManager = PinningTrustManager(pin)
        val context = SSLContext.getInstance("TLS")
        context.init(null, arrayOf(trustManager), SecureRandom())
        val http = base.newBuilder()
            .sslSocketFactory(context.socketFactory, trustManager)
            .hostnameVerifier(anyHostname)
            .build()
        return PinnedClient(http, trustManager)
    }

    /** Walks the cause chain: the TLS stack wraps the trust manager's exception. */
    fun isPinMismatch(t: Throwable): Boolean {
        var current: Throwable? = t
        var depth = 0
        while (current != null && depth < 16) {
            if (current is PinMismatchException) return true
            current = current.cause
            depth++
        }
        return false
    }
}
