package com.clipboardshare

/**
 * Protocol check against the PC (#12). /health and /pair/request report the server's
 * `protocol`; the app pairs and syncs only when it equals [APP_PROTOCOL]. A missing field
 * means a server older than the protocol field itself.
 */
object ProtocolCompat {
    /** The HTTP API protocol this app speaks. Bump it together with the server's. */
    const val APP_PROTOCOL = 1

    enum class Result { COMPATIBLE, SERVER_OLDER, SERVER_NEWER }

    /** Outcome of one GET /health before connecting or syncing. */
    sealed class HealthProbe {
        /** Network error, non-2xx or certificate problem: the caller behaves as before. */
        object Unreachable : HealthProbe()

        /** /health answered; [protocol] is null when the field is missing or not a number. */
        data class Answered(val protocol: Int?) : HealthProbe()
    }

    /**
     * The message to show instead of connecting or syncing, or null to go ahead. Only an
     * answer with another (or no) protocol blocks; an unreachable PC keeps today's flow.
     */
    fun blockingMessage(probe: HealthProbe, appProtocol: Int = APP_PROTOCOL): String? = when (probe) {
        HealthProbe.Unreachable -> null
        is HealthProbe.Answered -> message(probe.protocol, appProtocol)
    }

    /**
     * The clipboard sync probes /health before its first poll and before the poll after a
     * failed one (the PC may have been restarted with another version); never otherwise.
     */
    fun probeBeforePoll(firstPoll: Boolean, lastPollReachedServer: Boolean): Boolean =
        firstPoll || !lastPollReachedServer

    fun check(serverProtocol: Int?, appProtocol: Int = APP_PROTOCOL): Result = when {
        serverProtocol == null || serverProtocol < appProtocol -> Result.SERVER_OLDER
        serverProtocol > appProtocol -> Result.SERVER_NEWER
        else -> Result.COMPATIBLE
    }

    /** The status-line message for an incompatible PC, or null when it is compatible. */
    fun message(serverProtocol: Int?, appProtocol: Int = APP_PROTOCOL): String? {
        val hint = when (check(serverProtocol, appProtocol)) {
            Result.COMPATIBLE -> return null
            Result.SERVER_OLDER -> "Actualizá Nudge en la PC."
            Result.SERVER_NEWER -> "Actualizá la app."
        }
        val server = serverProtocol?.toString() ?: "desconocido"
        return "La PC usa otra versión de Nudge (protocolo $server; esta app usa $appProtocol). " +
            "Actualizá la app o el servidor para que coincidan. $hint"
    }

    /**
     * The `protocol` JSON value (as returned by `JSONObject.opt`) as an Int; null when it
     * is missing or not an integral number.
     */
    fun protocolOf(raw: Any?): Int? = when (raw) {
        is Int -> raw
        is Long -> if (raw in Int.MIN_VALUE..Int.MAX_VALUE) raw.toInt() else null
        is Double -> if (raw % 1.0 == 0.0 && raw >= Int.MIN_VALUE && raw <= Int.MAX_VALUE) raw.toInt() else null
        else -> null
    }
}
