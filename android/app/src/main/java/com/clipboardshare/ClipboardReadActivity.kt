package com.clipboardshare

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.widget.Toast

/**
 * Transparent, UI-less activity opened by the "Enviar portapapeles" Quick Settings tile
 * or notification action. Android 10+ lets only the focused app read the clipboard, so
 * this window reads it once it has focus, hands the content to the sync service and
 * finishes. Sending to the PC happens in the service after this window is gone, so a
 * slow or unreachable PC can never keep a full-screen window over the user's touches.
 */
class ClipboardReadActivity : Activity() {

    private val handler = Handler(Looper.getMainLooper())
    private var handled = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Hard cap that is never cancelled: whatever happens (no focus, service gone,
        // a slow clipboard provider), this window closes.
        handler.postDelayed({ finishQuietly("closed (time limit)") }, READ_WINDOW_LIMIT_MS)
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (!hasFocus || handled) return
        handled = true
        val service = ClipboardSyncService.instance
        if (service == null) {
            Toast.makeText(
                this,
                ClipboardSendOutcome.NO_SYNC.statusText(""),
                Toast.LENGTH_SHORT,
            ).show()
            finishQuietly("abort (service not running)")
            return
        }
        // The callback fires as soon as the clipboard has been read, before any network.
        service.captureLocalClipAndSend { runOnUiThread { finishQuietly(null) } }
    }

    private fun finishQuietly(reason: String?) {
        if (isFinishing) return
        if (reason != null) Log.d(TAG, "manual send: $reason")
        finish()
        @Suppress("DEPRECATION")
        overridePendingTransition(0, 0)
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        super.onDestroy()
    }

    companion object {
        private const val TAG = "ClipboardShare"

        /** Hard cap on how long this window may stay on screen. */
        private const val READ_WINDOW_LIMIT_MS = 3_000L

        /** Intent that opens the read window from a tile, notification or service. */
        fun intent(context: Context): Intent =
            Intent(context, ClipboardReadActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_NO_ANIMATION)
    }
}
