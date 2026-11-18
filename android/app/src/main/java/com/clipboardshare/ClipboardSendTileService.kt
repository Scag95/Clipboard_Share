package com.clipboardshare

import android.app.PendingIntent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService

/**
 * Quick Settings tile "Enviar portapapeles": one tap sends the phone's current
 * clipboard to the PC, like KDE Connect. The tile cannot read the clipboard itself
 * (it never has focus), so it opens the transparent [ClipboardReadActivity].
 * TileService exists since API 24, the app's minSdk.
 */
class ClipboardSendTileService : TileService() {

    override fun onStartListening() {
        super.onStartListening()
        val tile = qsTile ?: return
        tile.state =
            if (ClipboardSyncService.instance != null) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.updateTile()
    }

    override fun onClick() {
        super.onClick()
        // On the lock screen the clipboard of the unlocked user is what should be sent.
        if (isLocked) unlockAndRun { openReadWindow() } else openReadWindow()
    }

    private fun openReadWindow() {
        val intent = ClipboardReadActivity.intent(this)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            // API 34+ only accepts the PendingIntent overload.
            val pending = PendingIntent.getActivity(
                this,
                0,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            startActivityAndCollapse(pending)
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(intent)
        }
    }
}
