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
 * has focus and finishes. Only the read needs focus: sending to the PC happens in the
 * service, after this window is gone, so a slow or unreachable PC can never keep a
 * full-screen window over the user's touches.
 */
class ClipboardReadActivity : Activity() {

    private val handler = Handler(Looper.getMainLooper())
    private var handled = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Hard cap that is never cancelled: whatever happens (no focus, service gone,
        // a slow clipboard provider), this window closes.
        handler.postDelayed(
            { finishQuietly("closed (time limit)") },
            ClipboardTriggerPolicy.READ_WINDOW_LIMIT_MS,
        )
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (!hasFocus || handled) return
        handled = true
        val service = ClipboardSyncService.instance
        if (service == null) {
            finishQuietly("abort (service not running)")
            return
        }
        // The callback fires as soon as the clipboard has been read, before any network.
        service.captureLocalClipAndSync { runOnUiThread { finishQuietly(null) } }
    }

    private fun finishQuietly(reason: String?) {
        if (isFinishing) return
        if (reason != null) Log.d(TAG, "bg read: $reason")
        finish()
        @Suppress("DEPRECATION")
        overridePendingTransition(0, 0)
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        ClipboardSyncService.readActivityFinished()
        super.onDestroy()
    }

    private companion object {
        const val TAG = "Nudge"
    }
}
