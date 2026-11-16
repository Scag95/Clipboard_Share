package com.clipboardshare

import android.content.Context
import android.os.Build
import java.util.UUID

/** Persistent device identity and the latest token approved for that identity. */
class SessionStore(context: Context) {
    private val prefs = context.getSharedPreferences("input_control", Context.MODE_PRIVATE)

    val deviceId: String = prefs.getString("device_id", null) ?: UUID.randomUUID().toString().also {
        prefs.edit().putString("device_id", it).apply()
    }

    val deviceName: String
        get() = Build.MODEL

    var token: String?
        get() = prefs.getString("token", null)
        set(value) {
            if (value == null) prefs.edit().remove("token").apply()
            else prefs.edit().putString("token", value).apply()
        }
}
