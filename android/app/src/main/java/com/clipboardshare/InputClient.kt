package com.clipboardshare

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONArray
import org.json.JSONObject
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
    interface Listener {
        fun onConnected()

        /** reason is "unauthorized" when the server rejected the session token. */
        fun onDisconnected(reason: String?)

        /** A single event was rejected by the server (connection stays open). */
        fun onEventRejected(message: String)
    }

    @Volatile
    private var ws: WebSocket? = null

    val connected: Boolean
        get() = ws != null

    fun connect(serverHttpUrl: String, token: String, listener: Listener) {
        if (ws != null) return
        val wsUrl = serverHttpUrl.replaceFirst("http", "ws") + "/ws/input?token=" + token
        val request = Request.Builder().url(wsUrl).build()
        ws = http.newWebSocket(request, object : WebSocketListener() {
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
                listener.onDisconnected(reason)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                ws = null
                listener.onDisconnected(
                    if (response?.code == 401) "unauthorized" else t.message
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

    fun sendText(chars: String): Boolean =
        send(JSONObject().put("type", "text").put("chars", chars.take(1000)))

    private fun send(event: JSONObject): Boolean = ws?.send(event.toString()) ?: false

    private fun clampDelta(v: Int): Int = v.coerceIn(-1000, 1000)

    private fun clampNotches(v: Int): Int = v.coerceIn(-100, 100)
}

/** Exchanges the pairing PIN for a session token (S4). */
object PairingClient {
    private val http by lazy {
        OkHttpClient.Builder().connectTimeout(3, TimeUnit.SECONDS)
            .readTimeout(5, TimeUnit.SECONDS).build()
    }

    /** Returns the session token, or null for a wrong/spent PIN or unreachable server. */
    fun pair(serverHttpUrl: String, pin: String): String? {
        val body = JSONObject().put("pin", pin).toString()
            .toRequestBody("application/json".toMediaType())
        val request = Request.Builder().url("$serverHttpUrl/pair").post(body).build()
        return try {
            http.newCall(request).execute().use { resp ->
                if (!resp.isSuccessful) return@use null
                val token = JSONObject(resp.body?.string() ?: return@use null)
                    .optString("token")
                if (token.isNotEmpty()) token else null
            }
        } catch (e: Exception) {
            null
        }
    }
}

/** Where the last discovered server lives; shared between the two screens. */
object ServerRepo {
    @Volatile
    var url: String? = null
}
