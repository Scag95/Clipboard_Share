package com.clipboardshare

import android.content.Context

/**
 * Persists the pairing session token. The token is only valid for the current
 * server process; a 401 on connect clears it and the PIN flow restarts.
 */
class SessionStore(context: Context) {
    private val prefs = context.getSharedPreferences("input_control", Context.MODE_PRIVATE)

    var token: String?
        get() = prefs.getString("token", null)
        set(value) {
            if (value == null) prefs.edit().remove("token").apply()
            else prefs.edit().putString("token", value).apply()
        }
}
