package com.clipboardshare

import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONException
import org.json.JSONObject
import java.util.concurrent.TimeUnit

/**
 * One GET /health to read the PC's `protocol` (#12). Never sends a token. Blocking:
 * call it off the main thread.
 */
object HealthCheck {
    /** Short timeouts for the probe, the same as the home screen's reachability probe. */
    val baseClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(1_500, TimeUnit.MILLISECONDS)
            .readTimeout(1_500, TimeUnit.MILLISECONDS)
            .writeTimeout(1_500, TimeUnit.MILLISECONDS)
            .callTimeout(1_500, TimeUnit.MILLISECONDS)
            .build()
    }

    /** [http] decides the trust (pinned, or the no-pin client when there is no pin). */
    fun probe(http: OkHttpClient, serverUrl: String): ProtocolCompat.HealthProbe = try {
        val request = Request.Builder().url("$serverUrl/health").get().build()
        http.newCall(request).execute().use { resp ->
            if (!resp.isSuccessful) return@use ProtocolCompat.HealthProbe.Unreachable
            val protocol = try {
                ProtocolCompat.protocolOf(JSONObject(resp.body?.string() ?: "").opt("protocol"))
            } catch (_: JSONException) {
                null
            }
            ProtocolCompat.HealthProbe.Answered(protocol)
        }
    } catch (e: Exception) {
        ProtocolCompat.HealthProbe.Unreachable
    }
}
