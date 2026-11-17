package com.clipboardshare

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ClipData
import android.content.ClipboardManager
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.os.Build
import android.Manifest
import android.content.pm.PackageManager
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import android.provider.Settings
import android.provider.MediaStore
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.util.concurrent.TimeUnit

/** Owns clipboard synchronization for the last selected host, independently of any screen. */
class ClipboardSyncService : Service() {

    companion object {
        private const val TAG = "ClipboardShare"
        private const val CHANNEL_ID = "clipboard_sync"
        private const val NOTIFICATION_ID = 1001
        private const val POLL_MS = 2_000L
        private const val DISCONNECTED_FAILURES = 3

        /** Process-local state read by the remote screen without a second sync loop. */
        @Volatile
        var clipboardStatus: String = "Sincronización inactiva"
            private set

        /**
         * Started activities of this app. Android 10+ lets only the focused app read
         * the clipboard, so the local side is only readable while this is above zero.
         */
        private val visibleActivities = java.util.concurrent.atomic.AtomicInteger(0)

        fun onActivityStarted() {
            visibleActivities.incrementAndGet()
        }

        fun onActivityStopped() {
            visibleActivities.updateAndGet { maxOf(0, it - 1) }
        }

        private fun appVisible(): Boolean = visibleActivities.get() > 0

        /** The live service, so the transient read activity can ask it to sync right now. */
        @Volatile
        var instance: ClipboardSyncService? = null
            private set

        private val readActivityInFlight = java.util.concurrent.atomic.AtomicBoolean(false)

        fun readActivityFinished() {
            readActivityInFlight.set(false)
        }

        private const val READ_ACTIVITY_GRACE_MS = 5_000L
        private const val OWN_CHANGE_QUIET_MS = 1_500L
        private const val TRIGGER_DEBOUNCE_MS = 400L
        private const val LOCAL_READ_PENDING_MS = 3_000L

        fun ensureStarted(context: Context) {
            val appContext = context.applicationContext
            ContextCompat.startForegroundService(
                appContext,
                Intent(appContext, ClipboardSyncService::class.java),
            )
        }
    }

    /** Snapshot of one clipboard content: either text or PNG bytes, not both. */
    private class ClipItem(val isImage: Boolean, val text: String?, val png: ByteArray?) {
        override fun equals(other: Any?): Boolean {
            if (this === other) return true
            if (other !is ClipItem) return false
            if (isImage != other.isImage || text != other.text) return false
            return png?.contentEquals(other.png ?: ByteArray(0)) ?: (other.png == null)
        }

        override fun hashCode(): Int =
            if (isImage) png?.contentHashCode() ?: 0 else text?.hashCode() ?: 0
    }

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val clipboardClient = OkHttpClient.Builder()
        .connectTimeout(3, TimeUnit.SECONDS)
        .readTimeout(10, TimeUnit.SECONDS)
        .build()
    private val clipPrefs by lazy { getSharedPreferences("clip_settings", Context.MODE_PRIVATE) }

    private lateinit var clipboardManager: ClipboardManager
    private var clipListener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private var syncStarted = false

    @Volatile private var serverUrl = ""
    @Volatile private var hostKey = ""
    @Volatile private var shareImagesEnabled = false

    /** Last content this device has already propagated in both directions for this host. */
    @Volatile private var lastSyncedItem: ClipItem? = null

    private var consecutiveNetworkFailures = 0
    private var notificationConnected = true

    private val mainHandler = Handler(Looper.getMainLooper())
    @Volatile private var logWatcher: Process? = null
    @Volatile private var logWatcherThread: Thread? = null
    @Volatile private var suppressTriggersUntil = 0L
    private var lastTriggerAt = 0L
    @Volatile private var localReadPendingUntil = 0L

    override fun onCreate() {
        super.onCreate()
        clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        instance = this
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val selectedHost = SessionStore(this).selectedHost?.takeIf { it.isNotBlank() }
        val selectedUrl = selectedHost?.let(::serverUrlForHostKey)
        if (selectedHost == null || selectedUrl == null) {
            Log.w(TAG, "Clipboard sync stopped: no valid selected host")
            clipboardStatus = "Sincronización inactiva"
            stopSelf()
            return START_STICKY
        }

        val hostChanged = hostKey != selectedHost
        hostKey = selectedHost
        serverUrl = selectedUrl
        if (hostChanged) {
            lastSyncedItem = null
            consecutiveNetworkFailures = 0
            notificationConnected = true
            clipboardStatus = "Conectando con $selectedHost…"
        }

        // A foreground notification is installed before any network or clipboard work.
        startForegroundWithNotification(connected = true)
        if (!syncStarted) {
            syncStarted = true
            Log.d(TAG, "Clipboard sync started for $selectedHost")
            startClipListener()
            startSyncLoop()
            startLogWatcher()
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        clipListener?.let(clipboardManager::removePrimaryClipChangedListener)
        clipListener = null
        syncStarted = false
        stopLogWatcher()
        instance = null
        serviceScope.cancel()
        clipboardStatus = "Sincronización inactiva"
        Log.d(TAG, "Clipboard sync stopped")
        super.onDestroy()
    }

    private fun serverUrlForHostKey(key: String): String? {
        val separator = key.lastIndexOf(':')
        if (separator <= 0 || separator == key.lastIndex) return null
        val host = key.substring(0, separator)
        val port = key.substring(separator + 1).toIntOrNull() ?: return null
        if (port !in 1..65535) return null
        val address = if (host.contains(':') && !host.startsWith("[")) "[$host]" else host
        return "http://$address:$port"
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "Clipboard Share",
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            setSound(null, null)
            enableVibration(false)
            description = "Estado de la sincronización del portapapeles"
        }
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    private fun buildNotification(connected: Boolean): Notification {
        val text = if (connected) {
            "Clipboard sincronizado con $hostKey"
        } else {
            "Sin conexión con $hostKey — reintentando"
        }
        val openApp = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_launcher)
            .setContentTitle("Clipboard Share")
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setContentIntent(openApp)
            .setOngoing(true)
            .setSilent(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .build()
    }

    private fun startForegroundWithNotification(connected: Boolean) {
        val notification = buildNotification(connected)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC,
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun updateNotification(connected: Boolean) {
        notificationConnected = connected
        getSystemService(NotificationManager::class.java)
            .notify(NOTIFICATION_ID, buildNotification(connected))
    }

    private fun startClipListener() {
        val listener = ClipboardManager.OnPrimaryClipChangedListener {
            // A copy on the device syncs immediately instead of waiting for the next poll.
            serviceScope.launch { runSync() }
        }
        clipListener = listener
        clipboardManager.addPrimaryClipChangedListener(listener)
    }

    private fun startSyncLoop() {
        serviceScope.launch {
            while (isActive) {
                recordPollResult(runSync())
                delay(POLL_MS)
            }
        }
    }

    /** Only polling outcomes control the three-failure notification threshold. */
    private fun recordPollResult(networkAvailable: Boolean) {
        if (networkAvailable) {
            if (consecutiveNetworkFailures >= DISCONNECTED_FAILURES) {
                Log.d(TAG, "Clipboard host reachable again: $hostKey")
            }
            consecutiveNetworkFailures = 0
            if (!notificationConnected) updateNotification(connected = true)
            return
        }
        consecutiveNetworkFailures++
        if (
            consecutiveNetworkFailures >= DISCONNECTED_FAILURES &&
            notificationConnected
        ) {
            Log.w(TAG, "Clipboard host unreachable: $hostKey")
            updateNotification(connected = false)
        }
    }

    /**
     * One sync step against the selected host. Local changes win over remote
     * ones; lastSyncedItem prevents content from echoing back to its source.
     * Returns whether this step reached the server, for notification retries.
     */
    private fun runSync(capturedLocal: ClipItem? = null): Boolean {
        val targetUrl = serverUrl
        val targetHost = hostKey
        if (targetUrl.isEmpty()) return false
        return try {
            // Read on every step so the UI checkbox takes effect without a restart.
            shareImagesEnabled = clipPrefs.getBoolean("share_images", false)
            // Android 10+ lets only the focused app read the clipboard. capturedLocal is
            // the item ClipboardReadActivity read while it held focus; otherwise do not
            // even try without focus: the denied read only returns null.
            val forceLocalRead = capturedLocal != null
            val canReadLocal = forceLocalRead || appVisible() ||
                Build.VERSION.SDK_INT < Build.VERSION_CODES.Q
            val statusSuffix = if (canReadLocal) "" else backgroundStatusSuffix()
            val local = capturedLocal ?: if (canReadLocal) readLocalClip() else null
            if (forceLocalRead && local != null) Log.d(TAG, "bg read: ${describeForLog(local)}")
            if (local != null && local != lastSyncedItem) {
                if (local.isImage && !shareImagesEnabled) {
                    // Do not let remote content overwrite an unsent local image.
                    lastSyncedItem = local
                    clipboardStatus = "Imagen local no compartida"
                    return true
                }
                if (!push(targetUrl, local)) {
                    // Retry on the next poll because the server did not accept it.
                    Log.e(TAG, "push failed; item not marked synced")
                    clipboardStatus = "Error al enviar el portapapeles"
                    return false
                }
                lastSyncedItem = local
                clipboardStatus = "Portapapeles sincronizado"
                return true
            }
            val remote = fetchRemoteClip(targetUrl)
            if (remote != null && remote != lastSyncedItem) {
                if (local?.isImage == true && !shareImagesEnabled) return true
                // A phone copy was just detected and is being read: it wins, so do not
                // overwrite it with the PC's clipboard before that read has finished.
                if (!forceLocalRead && SystemClock.elapsedRealtime() < localReadPendingUntil) return true
                applyRemote(remote)
                lastSyncedItem = remote
                clipboardStatus = "Portapapeles sincronizado$statusSuffix"
            } else {
                clipboardStatus = "Sincronización activa$statusSuffix"
            }
            true
        } catch (e: Exception) {
            Log.e(TAG, "Sync error with $targetHost: ${e.message}")
            clipboardStatus = "Sin conexión con $targetHost"
            false
        }
    }

    /** Our own writes also make the system log a denial for us; those must not trigger a read. */
    private fun setClipQuietly(clip: ClipData) {
        suppressTriggersUntil = SystemClock.elapsedRealtime() + OWN_CHANGE_QUIET_MS
        clipboardManager.setPrimaryClip(clip)
    }

    /**
     * Describes a clip for the log without its content: copied text can be a password or
     * a one-time code, and logcat is readable by adb, bug reports and READ_LOGS apps.
     */
    private fun describeForLog(item: ClipItem): String =
        if (item.isImage) "<image ${item.png?.size ?: 0} bytes>"
        else "<text ${item.text?.length ?: 0} chars>"

    /** Empty when background phone -> PC is fully set up; otherwise names what is missing. */
    private fun backgroundStatusSuffix(): String = when {
        checkSelfPermission(Manifest.permission.READ_LOGS) != PackageManager.PERMISSION_GRANTED ->
            " · Falta permiso READ_LOGS: ver README"
        !Settings.canDrawOverlays(this) -> " · Falta permiso de superposición: ver README"
        else -> ""
    }

    /**
     * Reads the system log for the line Android prints when a clipboard change is
     * dispatched to a listener without focus. That line is the only signal an unfocused
     * app gets that the user copied something.
     */
    private fun startLogWatcher() {
        if (checkSelfPermission(Manifest.permission.READ_LOGS) != PackageManager.PERMISSION_GRANTED) {
            Log.w(TAG, "bg trigger disabled: READ_LOGS not granted")
            return
        }
        if (logWatcherThread?.isAlive == true) return
        val thread = Thread({
            var backoffMs = 1_000L
            while (!Thread.currentThread().isInterrupted) {
                try {
                    val process = ProcessBuilder(
                        // ClipboardService runs in system_server, whose log goes to the system buffer.
                        "logcat", "-v", "brief", "-b", "system", "-T", "1", "ClipboardService:E", "*:S",
                    ).redirectErrorStream(true).start()
                    logWatcher = process
                    Log.d(TAG, "bg watcher started")
                    process.inputStream.bufferedReader().useLines { lines ->
                        for (line in lines) {
                            backoffMs = 1_000L
                            if (line.contains("Denying clipboard access to $packageName")) {
                                onClipboardChangeDetected()
                            }
                        }
                    }
                } catch (e: InterruptedException) {
                    return@Thread
                } catch (e: Exception) {
                    Log.w(TAG, "logcat watcher error: ${e.message}")
                }
                if (Thread.currentThread().isInterrupted) return@Thread
                Log.w(TAG, "logcat watcher ended (exit=${runCatching { logWatcher?.exitValue() }.getOrNull()}); restarting in ${backoffMs}ms")
                try {
                    Thread.sleep(backoffMs)
                } catch (_: InterruptedException) {
                    return@Thread
                }
                backoffMs = minOf(backoffMs * 2, 30_000L)
            }
        }, "clipboard-log-watcher")
        thread.isDaemon = true
        logWatcherThread = thread
        thread.start()
    }

    private fun stopLogWatcher() {
        logWatcherThread?.interrupt()
        logWatcherThread = null
        logWatcher?.destroy()
        logWatcher = null
    }

    private fun onClipboardChangeDetected() {
        if (appVisible()) return
        val now = SystemClock.elapsedRealtime()
        if (now < suppressTriggersUntil) return
        if (now - lastTriggerAt < TRIGGER_DEBOUNCE_MS) return
        lastTriggerAt = now
        Log.d(TAG, "bg trigger: clipboard changed")
        if (!Settings.canDrawOverlays(this)) {
            Log.w(TAG, "bg trigger ignored: overlay permission missing")
            return
        }
        if (!readActivityInFlight.compareAndSet(false, true)) return
        mainHandler.postDelayed({ readActivityInFlight.set(false) }, READ_ACTIVITY_GRACE_MS)
        // The phone copy wins: hold the PC's clipboard back until it has been read and sent.
        localReadPendingUntil = now + LOCAL_READ_PENDING_MS
        try {
            startActivity(
                Intent(this, ClipboardReadActivity::class.java)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_NO_ANIMATION),
            )
        } catch (e: Exception) {
            Log.e(TAG, "bg read activity failed: ${e.message}")
            readActivityInFlight.set(false)
            localReadPendingUntil = 0L
        }
    }

    /**
     * Called by ClipboardReadActivity once it holds focus, so the read is allowed.
     * [onCaptured] runs right after the clipboard has been read and before any network
     * work, so the window can close at once; the push to the PC happens afterwards.
     */
    fun captureLocalClipAndSync(onCaptured: () -> Unit) {
        serviceScope.launch {
            val item = try {
                readLocalClip()
            } catch (e: Exception) {
                Log.e(TAG, "bg read failed: ${e.message}")
                null
            }
            onCaptured()
            try {
                recordPollResult(runSync(capturedLocal = item))
            } finally {
                localReadPendingUntil = 0L
            }
        }
    }

    private fun readLocalClip(): ClipItem? {
        val clip = clipboardManager.primaryClip ?: return null
        if (clip.itemCount == 0) return null
        val item = clip.getItemAt(0)
        val isImage = clip.description.hasMimeType("image/*")
        if (isImage) {
            val uri = item.uri ?: return null
            val bmp = try {
                contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it) }
            } catch (e: Exception) {
                Log.e(TAG, "Cannot read local image: ${e.message}")
                null
            } ?: return null
            val out = ByteArrayOutputStream()
            bmp.compress(Bitmap.CompressFormat.PNG, 100, out)
            bmp.recycle()
            return ClipItem(true, null, out.toByteArray())
        }
        val text = item.coerceToText(this).toString()
        if (text.isEmpty()) return null
        return ClipItem(false, text, null)
    }

    /** Push returns true only when the server accepted the content (2xx). */
    private fun push(url: String, item: ClipItem): Boolean {
        return if (item.isImage) {
            setImage(url, item.png ?: ByteArray(0))
        } else {
            setText(url, item.text ?: "")
        }
    }

    private fun applyRemote(item: ClipItem) {
        if (item.isImage) {
            val bytes = item.png ?: ByteArray(0)
            val uri = saveImageToMediaStore(bytes) ?: return
            setClipQuietly(ClipData.newUri(contentResolver, "shared", uri))
            Log.d(TAG, "remote applied: <image ${bytes.size} bytes>")
        } else {
            val text = item.text ?: ""
            setClipQuietly(ClipData.newPlainText("shared", text))
            Log.d(TAG, "remote applied: ${describeForLog(item)}")
        }
    }

    private fun saveImageToMediaStore(png: ByteArray): Uri? {
        val values = ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, "clipboard_share_${System.currentTimeMillis()}.png")
            put(MediaStore.Images.Media.MIME_TYPE, "image/png")
        }
        val uri = contentResolver.insert(
            MediaStore.Images.Media.EXTERNAL_CONTENT_URI,
            values,
        ) ?: return null
        return try {
            contentResolver.openOutputStream(uri)?.use { it.write(png) }
            uri
        } catch (e: Exception) {
            Log.e(TAG, "Cannot save image: ${e.message}")
            contentResolver.delete(uri, null, null)
            null
        }
    }

    private fun fetchRemoteClip(url: String): ClipItem? {
        if (shareImagesEnabled) {
            getImage(url)?.let { if (it.isNotEmpty()) return ClipItem(true, null, it) }
        }
        val text = getText(url)
        if (!text.isNullOrEmpty()) return ClipItem(false, text, null)
        return null
    }

    private fun getText(url: String): String? {
        val req = Request.Builder().url("$url/clipboard/text").get().build()
        clipboardClient.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return null
            val body = resp.body?.string() ?: return null
            return JSONObject(body).optString("text", "")
        }
    }

    private fun setText(url: String, text: String): Boolean {
        val json = JSONObject().put("text", text).toString()
        val body = json.toRequestBody("application/json".toMediaType())
        val req = Request.Builder().url("$url/clipboard/text").post(body).build()
        clipboardClient.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) {
                Log.e(TAG, "setText push: HTTP ${resp.code}")
                return false
            }
            return true
        }
    }

    private fun getImage(url: String): ByteArray? {
        val req = Request.Builder().url("$url/clipboard/image").get().build()
        clipboardClient.newCall(req).execute().use { resp ->
            if (resp.code == 204) return null
            if (!resp.isSuccessful) return null
            return resp.body?.bytes()
        }
    }

    private fun setImage(url: String, data: ByteArray): Boolean {
        val body = data.toRequestBody("image/png".toMediaType())
        val req = Request.Builder().url("$url/clipboard/image").post(body).build()
        clipboardClient.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) {
                Log.e(TAG, "setImage push: HTTP ${resp.code}")
                return false
            }
            return true
        }
    }
}
