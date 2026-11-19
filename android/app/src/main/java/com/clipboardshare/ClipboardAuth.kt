package com.clipboardshare

/**
 * Token header and response decisions for the clipboard HTTP calls (/clipboard/text,
 * /clipboard/image). The token goes in a header, never in the URL, which ends up in logs.
 */
object ClipboardAuth {
    const val HEADER_NAME = "Authorization"
    const val REPAIR_STATUS = "La PC rechazó este teléfono. Emparejalo de nuevo."

    /** What the sync does after a clipboard response. */
    enum class Outcome {
        /** 2xx, including 204 "no image". */
        OK,

        /** 401: the PC no longer accepts this phone's token; it has to pair again. */
        REPAIR,

        /** Any other code: retried on the next poll, as before. */
        TRANSIENT_ERROR,
    }

    /** `Bearer <token>`, or null when there is no usable token and no header must be sent. */
    fun headerValue(token: String?): String? =
        if (token.isNullOrBlank()) null else "Bearer $token"

    /** The clipboard sync only runs while the selected host has a token. */
    fun canSync(token: String?): Boolean = headerValue(token) != null

    fun classify(code: Int): Outcome = when {
        code == 401 -> Outcome.REPAIR
        code in 200..299 -> Outcome.OK
        else -> Outcome.TRANSIENT_ERROR
    }
}
