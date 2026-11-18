package com.clipboardshare

import android.content.Context
import android.os.Build
import org.json.JSONArray
import org.json.JSONObject
import java.util.UUID

/** Persistent device identity, per-host session tokens, and remembered hosts. */
class SessionStore(context: Context) {
    data class LastHost(
        val name: String,
        val host: String,
        val port: Int,
        val lastConnected: Long,
    )

    companion object {
        private const val LAST_HOSTS = "last_hosts"
        private const val SELECTED_HOST = "selected_host"
        private const val TOKEN_PREFIX = "token_"

        fun hostKey(host: String, port: Int): String = "$host:$port"
    }

    private val prefs = context.getSharedPreferences("input_control", Context.MODE_PRIVATE)

    val deviceId: String = prefs.getString("device_id", null) ?: UUID.randomUUID().toString().also {
        prefs.edit().putString("device_id", it).apply()
    }

    val deviceName: String
        get() = Build.MODEL

    var selectedHost: String?
        get() = prefs.getString(SELECTED_HOST, null)
        set(value) {
            if (value == null) prefs.edit().remove(SELECTED_HOST).apply()
            else prefs.edit().putString(SELECTED_HOST, value).apply()
        }

    fun getToken(hostKey: String): String? = prefs.getString(TOKEN_PREFIX + hostKey, null)

    fun putToken(hostKey: String, token: String) {
        prefs.edit().putString(TOKEN_PREFIX + hostKey, token).apply()
    }

    fun clearToken(hostKey: String) {
        prefs.edit().remove(TOKEN_PREFIX + hostKey).apply()
    }

    fun getLastHosts(): List<LastHost> {
        val raw = prefs.getString(LAST_HOSTS, "[]") ?: "[]"
        return try {
            val json = JSONArray(raw)
            buildList {
                for (index in 0 until json.length()) {
                    val item = json.optJSONObject(index) ?: continue
                    val host = item.optString("host")
                    val port = item.optInt("port", 0)
                    if (!HostAddress.isUsable(host) || port <= 0) continue
                    add(
                        LastHost(
                            name = item.optString("name").ifEmpty { host },
                            host = host,
                            port = port,
                            lastConnected = item.optLong("lastConnected", 0L),
                        )
                    )
                }
            }
        } catch (_: Exception) {
            emptyList()
        }
    }

    /** Adds or refreshes a host at the front of the list and records the selection. */
    fun putLastHost(host: LastHost) {
        val key = hostKey(host.host, host.port)
        val updated = getLastHosts().filterNot { hostKey(it.host, it.port) == key }.toMutableList()
        updated.add(0, host)
        val json = JSONArray()
        updated.forEach { remembered ->
            json.put(
                JSONObject()
                    .put("name", remembered.name)
                    .put("host", remembered.host)
                    .put("port", remembered.port)
                    .put("lastConnected", remembered.lastConnected)
            )
        }
        prefs.edit()
            .putString(LAST_HOSTS, json.toString())
            .putString(SELECTED_HOST, key)
            .apply()
    }
}
