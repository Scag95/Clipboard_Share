package com.clipboardshare

/**
 * Whether /pair/request carries the phone's current token (#21). The PC renews a known
 * phone's token without asking only when it presents that token; it is sent only when the
 * PC also has a stored pin, so it travels solely over the pinned connection and never in
 * pairing (TOFU) mode.
 */
object PairingAuth {
    /** `Bearer <token>` when both [token] and [pin] are stored for the PC, else null (no header). */
    fun requestHeaderValue(token: String?, pin: String?): String? =
        if (pin.isNullOrBlank()) null else ClipboardAuth.headerValue(token)
}
