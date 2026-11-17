package com.clipboardshare

import android.app.Activity
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log

/**
 * Transparent, UI-less activity that exists for a fraction of a second. Android 10+
 * lets only the focused app read the clipboard, so when the service detects a copy
 * made elsewhere it brings this window to the front, reads the clipboard while it
 * has focus, sends it to the PC and finishes.
 */
class ClipboardReadActivity : Activity() {

    private val handler = Handler(Looper.getMainLooper())
    private var handled = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Focus normally arrives within a few hundred milliseconds; do not linger.
        handler.postDelayed({ finishQuietly("abort (no focus)") }, FOCUS_TIMEOUT_MS)
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (!hasFocus || handled) return
        handled = true
        handler.removeCallbacksAndMessages(null)
        val service = ClipboardSyncService.instance
        if (service == null) {
            finishQuietly("abort (service not running)")
            return
        }
        // The service reads the clipboard now, while this window holds focus.
        service.syncLocalNow { runOnUiThread { finishQuietly(null) } }
    }

    private fun finishQuietly(reason: String?) {
        if (reason != null) Log.d(TAG, "bg read: $reason")
        if (!isFinishing) {
            finish()
            @Suppress("DEPRECATION")
            overridePendingTransition(0, 0)
        }
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        ClipboardSyncService.readActivityFinished()
        super.onDestroy()
    }

    private companion object {
        const val TAG = "ClipboardShare"
        const val FOCUS_TIMEOUT_MS = 1_500L
    }
}
