package com.clipboardshare

import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONArray
import org.json.JSONObject
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit

/**
 * WebSocket client for the remote-control event stream (S3). One connection
 * carries validated S6 events; the server never injects what it rejects and
 * answers invalid frames with {"error": ...}.
 */
class InputClient(
    private val http: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(3, TimeUnit.SECONDS)
        .pingInterval(20, TimeUnit.SECONDS)
        .build(),
) {
    companion object {
        /** onDisconnected reason when the PC presented a certificate other than the pin. */
        const val PIN_MISMATCH = "pin_mismatch"
    }

    interface Listener {
        fun onConnected()

        /**
         * reason is "unauthorized" when the server rejected the session token and
         * [PIN_MISMATCH] when the PC's certificate is not the pinned one.
         */
        fun onDisconnected(reason: String?)

        /** A single event was rejected by the server (connection stays open). */
        fun onEventRejected(message: String)
    }

    @Volatile
    private var ws: WebSocket? = null

    val connected: Boolean
        get() = ws != null

    /** Opens the wss socket, accepting only the certificate whose fingerprint is [pin]. */
    fun connect(serverUrl: String, token: String, pin: String, listener: Listener) {
        if (ws != null) return
        val pinned = PinnedTls.client(http, pin)
        val wsUrl = HostAddress.webSocketUrl(serverUrl, "/ws/input") + "?token=" + token
        val request = try {
            Request.Builder().url(wsUrl).build()
        } catch (e: IllegalArgumentException) {
            listener.onDisconnected(e.message)
            return
        }
        ws = pinned.http.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                listener.onConnected()
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                val error = JSONObject(text).optString("error", "")
                if (error.isNotEmpty()) listener.onEventRejected(error)
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                ws = null
                listener.onDisconnected(if (code == 1008) "unauthorized" else reason)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                ws = null
                listener.onDisconnected(
                    when {
                        pinned.isMismatch(t) -> PIN_MISMATCH
                        response?.code == 401 -> "unauthorized"
                        else -> t.message
                    }
                )
            }
        })
    }

    fun disconnect() {
        val w = ws
        ws = null
        w?.close(1000, "client goodbye")
    }

    /** Relative pointer move in pixels (server clamps magnitude to 1000). */
    fun sendMove(dx: Int, dy: Int): Boolean =
        send(JSONObject().put("type", "move").put("dx", clampDelta(dx)).put("dy", clampDelta(dy)))

    fun sendClick(button: String, mode: String): Boolean =
        send(JSONObject().put("type", "click").put("button", button).put("mode", mode))

    fun sendScroll(dx: Int, dy: Int): Boolean =
        send(JSONObject().put("type", "scroll").put("dx", clampNotches(dx)).put("dy", clampNotches(dy)))

    fun sendKey(key: String, mods: List<String> = emptyList()): Boolean =
        send(
            JSONObject().put("type", "key").put("key", key)
                .put("mods", JSONArray(mods))
        )

    /**
     * Sends unicode text (S6). Rejects — never silently truncates — strings
     * over 1000 code points or with control characters, so callers only
     * clear their input on a real send.
     */
    fun sendText(chars: String): Boolean {
        if (chars.isEmpty()) return false
        if (chars.any { it.code < 0x20 || it.code == 0x7f }) return false
        if (Character.codePointCount(chars, 0, chars.length) > 1000) return false
        return send(JSONObject().put("type", "text").put("chars", chars))
    }

    private fun send(event: JSONObject): Boolean = ws?.send(event.toString()) ?: false

    private fun clampDelta(v: Int): Int = v.coerceIn(-1000, 1000)

    private fun clampNotches(v: Int): Int = v.coerceIn(-100, 100)
}

/**
 * Device-identity pairing requests and approval polling (S3/S5), over TLS (S6).
 * [request] runs against the stored pin, or in pairing mode captures the certificate
 * the PC presents; [status] is then bound to exactly that certificate's fingerprint.
 */
object PairingClient {
    /** [fingerprint] is the certificate that answered: the pin to store on approval. */
    sealed class RequestResult {
        data class Approved(val token: String, val fingerprint: String) : RequestResult()
        data class Pending(val requestId: String, val fingerprint: String) : RequestResult()
        object TooManyRequests : RequestResult()
        object PinMismatch : RequestResult()
        object Failed : RequestResult()
    }

    sealed class StatusResult {
        object Pending : StatusResult()
        data class Approved(val token: String) : StatusResult()
        object Denied : StatusResult()
        object Expired : StatusResult()
        object PinMismatch : StatusResult()
        object Failed : StatusResult()
    }

    private val http by lazy {
        OkHttpClient.Builder().connectTimeout(3, TimeUnit.SECONDS)
            .readTimeout(5, TimeUnit.SECONDS).build()
    }

    /**
     * Asks the PC for access. With a [pin] (a known PC renewing its token) only that
     * certificate is accepted; with none, the presented certificate is captured.
     */
    fun request(serverUrl: String, pin: String?, deviceId: String, deviceName: String): RequestResult {
        val body = JSONObject().put("id", deviceId).put("name", deviceName).toString()
            .toRequestBody("application/json".toMediaType())
        // A fresh SSLContext per request: the handshake is always checked, never resumed.
        val pinned = PinnedTls.client(http, pin)
        return try {
            val request = Request.Builder().url("$serverUrl/pair/request").post(body).build()
            pinned.http.newCall(request).execute().use { resp ->
                val seen = seenFingerprint(resp, pinned) ?: return@use RequestResult.Failed
                when (resp.code) {
                    200 -> {
                        val token = JSONObject(resp.body?.string() ?: "").optString("token")
                        if (token.isNotEmpty()) RequestResult.Approved(token, seen) else RequestResult.Failed
                    }
                    202 -> {
                        val id = JSONObject(resp.body?.string() ?: "").optString("requestId")
                        if (id.isNotEmpty()) RequestResult.Pending(id, seen) else RequestResult.Failed
                    }
                    429 -> RequestResult.TooManyRequests
                    else -> RequestResult.Failed
                }
            }
        } catch (e: Exception) {
            if (pinned.isMismatch(e)) RequestResult.PinMismatch else RequestResult.Failed
        }
    }

    /** Polls the approval, accepting only the certificate [request] saw ([fingerprint]). */
    fun status(serverUrl: String, requestId: String, fingerprint: String): StatusResult {
        val pinned = PinnedTls.client(http, fingerprint)
        return try {
            val url = "$serverUrl/pair/status".toHttpUrl().newBuilder()
                .addQueryParameter("id", requestId).build()
            val request = Request.Builder().url(url).get().build()
            pinned.http.newCall(request).execute().use { resp ->
                // The server answers an expired request with HTTP 410 and
                // {"state":"expired"}; both spellings map to Expired so the
                // UI can ask the user to tap Conectar again.
                if (resp.code == 410) return@use StatusResult.Expired
                if (!resp.isSuccessful) return@use StatusResult.Failed
                val json = JSONObject(resp.body?.string() ?: "")
                when (json.optString("state")) {
                    "pending" -> StatusResult.Pending
                    "approved" -> {
                        val token = json.optString("token")
                        // The server returns this token only once; the caller stores it immediately.
                        if (token.isNotEmpty()) StatusResult.Approved(token) else StatusResult.Failed
                    }
                    "denied" -> StatusResult.Denied
                    "expired" -> StatusResult.Expired
                    else -> StatusResult.Failed
                }
            }
        } catch (e: Exception) {
            if (pinned.isMismatch(e)) StatusResult.PinMismatch else StatusResult.Failed
        }
    }

    /** Fingerprint of the certificate that served [resp]; the trust manager saw the same one. */
    private fun seenFingerprint(resp: Response, pinned: PinnedClient): String? {
        val leaf = resp.handshake?.peerCertificates?.firstOrNull() as? X509Certificate
        val seen = leaf?.let(CertPin::fingerprint) ?: pinned.trustManager.seenFingerprint
        // A pinned request can only have been answered by the pinned certificate.
        if (seen == null || CertPin.decide(pinned.trustManager.pin, seen) == CertPin.Decision.MISMATCH) {
            return null
        }
        return seen
    }
}
