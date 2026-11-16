package com.clipboardshare

import android.content.ClipData
import android.content.ClipboardManager
import android.content.ContentValues
import android.content.Context
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.MediaStore
import android.util.Log
import android.view.View
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.util.concurrent.TimeUnit

/**
 * Remote mouse, keyboard, and clipboard for the host selected on the home screen.
 * Approved per-host tokens reconnect without user interaction.
 */
class RemoteControlActivity : AppCompatActivity() {

    private companion object {
        const val TAG = "ClipboardShareRemote"
        const val POLL_MS = 2_000L
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

    private val client = InputClient()
    private val handler = Handler(Looper.getMainLooper())
    private lateinit var store: SessionStore
    private lateinit var serverUrl: String
    private lateinit var hostKey: String
    @Volatile private var manualDisconnect = false
    private var pairingJob: Job? = null

    private lateinit var statusText: TextView
    private lateinit var clipboardStatusText: TextView
    private lateinit var textInput: EditText
    private lateinit var connectButton: Button
    private lateinit var cancelPairingButton: Button
    private var retryScheduled = false
    private var authRetryInFlight = false

    private val clipboardClient = OkHttpClient.Builder()
        .connectTimeout(3, TimeUnit.SECONDS)
        .readTimeout(10, TimeUnit.SECONDS)
        .build()
    private var clipListener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private lateinit var shareImages: CheckBox
    private val clipPrefs by lazy { getSharedPreferences("clip_settings", Context.MODE_PRIVATE) }

    /** Opt-in shared globally through the existing clip_settings/share_images preference. */
    @Volatile private var shareImagesEnabled = false

    /** Last content this device has already propagated in both directions for this host. */
    @Volatile private var lastSyncedItem: ClipItem? = null

    private val wsListener = object : InputClient.Listener {
        override fun onConnected() {
            runOnUiThread {
                authRetryInFlight = false
                handler.removeCallbacksAndMessages(null)
                retryScheduled = false
                hidePairingControls()
                setStatus("Conectado a $serverUrl como ${store.deviceName}")
            }
        }

        override fun onDisconnected(reason: String?) {
            runOnUiThread {
                if (manualDisconnect) {
                    showConnect("Desconectado (a mano)")
                } else if (reason == "unauthorized") {
                    if (authRetryInFlight) {
                        // The replacement token was also rejected: require an explicit request.
                        authRetryInFlight = false
                        store.clearToken(hostKey)
                        showConnect("El PC rechazó la sesión. Toca Conectar para solicitar acceso")
                    } else {
                        // A known device receives a fresh token here without another approval.
                        requestPairing(automatic = true)
                    }
                } else if (store.getToken(hostKey) == null) {
                    showConnect("Dispositivo: ${store.deviceName}. Toca Conectar para solicitar acceso")
                } else {
                    hidePairingControls()
                    setStatus("Desconectado: $reason")
                    scheduleRetry()
                }
            }
        }

        override fun onEventRejected(message: String) {
            runOnUiThread { setStatus("Server rechazó un evento: $message") }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val selectedUrl = intent.getStringExtra("server_url")?.trimEnd('/')
        if (selectedUrl.isNullOrEmpty()) {
            Log.e(TAG, "RemoteControlActivity requires the server_url extra")
            finish()
            return
        }
        serverUrl = selectedUrl
        hostKey = serverUrl.substringAfter("://")
        store = SessionStore(this)
        store.selectedHost = hostKey

        setContentView(R.layout.activity_remote_control)
        statusText = findViewById(R.id.rcStatus)
        clipboardStatusText = findViewById(R.id.clipboardStatus)
        textInput = findViewById(R.id.etText)
        connectButton = findViewById(R.id.btnConnect)
        cancelPairingButton = findViewById(R.id.btnCancelPairing)

        shareImages = findViewById(R.id.cbShareImages)
        shareImagesEnabled = clipPrefs.getBoolean("share_images", false)
        shareImages.isChecked = shareImagesEnabled
        shareImages.setOnCheckedChangeListener { _, checked ->
            shareImagesEnabled = checked
            clipPrefs.edit().putBoolean("share_images", checked).apply()
        }

        findViewById<TouchpadView>(R.id.touchpad).listener = object : TouchpadView.Listener {
            override fun onMove(dx: Int, dy: Int) { client.sendMove(dx, dy) }
            override fun onLeftTap() { client.sendClick("left", "tap") }
            override fun onRightTap() { client.sendClick("right", "tap") }
            override fun onScroll(dxNotches: Int, dyNotches: Int) {
                client.sendScroll(dxNotches, dyNotches)
            }
        }

        bind(R.id.btnClickLeft) { client.sendClick("left", "tap") }
        bind(R.id.btnClickRight) { client.sendClick("right", "tap") }
        bind(R.id.btnCtrlC) { client.sendKey("c", listOf("ctrl")) }
        bind(R.id.btnCtrlV) { client.sendKey("v", listOf("ctrl")) }
        bind(R.id.btnCtrlX) { client.sendKey("x", listOf("ctrl")) }
        bind(R.id.btnCtrlZ) { client.sendKey("z", listOf("ctrl")) }
        bind(R.id.btnEsc) { client.sendKey("esc") }
        bind(R.id.btnEnter) { client.sendKey("enter") }
        bind(R.id.btnArrowLeft) { client.sendKey("arrowleft") }
        bind(R.id.btnArrowRight) { client.sendKey("arrowright") }
        bind(R.id.btnArrowUp) { client.sendKey("arrowup") }
        bind(R.id.btnArrowDown) { client.sendKey("arrowdown") }

        bind(R.id.btnSendText) {
            val chars = textInput.text.toString()
            when {
                chars.isEmpty() -> setStatus("Nada que enviar")
                !client.sendText(chars) -> setStatus(
                    if (client.connected) {
                        "Texto demasiado largo o con caracteres de control: no se envió"
                    } else {
                        "Sin conexión: el texto no se envió"
                    }
                )
                else -> textInput.setText("")
            }
        }

        connectButton.setOnClickListener { requestPairing(automatic = false) }
        cancelPairingButton.setOnClickListener { cancelPairing() }
        bind(R.id.btnDisconnect) {
            manualDisconnect = true
            pairingJob?.cancel()
            pairingJob = null
            handler.removeCallbacksAndMessages(null)
            retryScheduled = false
            client.disconnect()
            showConnect("Desconectado (a mano)")
        }

        setStatus("Preparando conexión con $serverUrl…")
        maybeRequestStoragePermission()
        startClipListener()
        startSyncLoop()
    }

    override fun onResume() {
        super.onResume()
        if (!::store.isInitialized || client.connected || pairingJob?.isActive == true) return
        manualDisconnect = false
        if (store.getToken(hostKey) != null) {
            connect()
        } else {
            requestPairing(automatic = false)
        }
    }

    override fun onDestroy() {
        manualDisconnect = true
        pairingJob?.cancel()
        handler.removeCallbacksAndMessages(null)
        client.disconnect()
        clipListener?.let { listener ->
            val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            cm.removePrimaryClipChangedListener(listener)
        }
        clipListener = null
        super.onDestroy()
    }

    private fun bind(id: Int, action: () -> Unit) {
        findViewById<Button>(id).setOnClickListener { action() }
    }

    /** Automatic connection using only the persisted token for the selected host. */
    private fun connect(isAuthRetry: Boolean = false) {
        manualDisconnect = false
        authRetryInFlight = isAuthRetry
        val token = store.getToken(hostKey)
        if (token == null) {
            showConnect("Dispositivo: ${store.deviceName}. Toca Conectar para solicitar acceso")
            return
        }
        if (client.connected) return
        hidePairingControls()
        setStatus("Conectando ${store.deviceName}…")
        client.connect(serverUrl, token, wsListener)
    }

    private fun scheduleRetry() {
        if (retryScheduled) return
        retryScheduled = true
        handler.postDelayed(
            {
                retryScheduled = false
                if (!client.connected && !isFinishing && store.getToken(hostKey) != null) connect()
            },
            2_500L,
        )
    }

    /** A manual retry creates a new request; automatic renewal is silent. */
    private fun requestPairing(automatic: Boolean) {
        manualDisconnect = false
        pairingJob?.cancel()
        handler.removeCallbacksAndMessages(null)
        retryScheduled = false
        hidePairingControls()
        setStatus(
            if (automatic) "Renovando acceso para ${store.deviceName}…"
            else "Solicitando acceso para ${store.deviceName}…"
        )
        pairingJob = lifecycleScope.launch {
            val result = withContext(Dispatchers.IO) {
                PairingClient.request(serverUrl, store.deviceId, store.deviceName)
            }
            when (result) {
                is PairingClient.RequestResult.Approved -> {
                    store.putToken(hostKey, result.token)
                    connect(isAuthRetry = automatic)
                }
                is PairingClient.RequestResult.Pending -> {
                    store.clearToken(hostKey)
                    connectButton.visibility = View.GONE
                    cancelPairingButton.visibility = View.VISIBLE
                    setStatus("Esperando aprobación en el PC para ${store.deviceName}…")
                    pollPairingStatus(serverUrl, result.requestId, automatic)
                }
                PairingClient.RequestResult.TooManyRequests -> showConnect(
                    "Hay demasiadas solicitudes pendientes. Espera y vuelve a intentar"
                )
                PairingClient.RequestResult.Failed -> showConnect(
                    if (automatic) "No se pudo renovar la sesión. Toca Conectar para reintentar"
                    else "No se pudo solicitar acceso. Verifica el servidor y reintenta"
                )
            }
        }
    }

    private suspend fun pollPairingStatus(url: String, requestId: String, automatic: Boolean) {
        while (lifecycleScope.coroutineContext.isActive) {
            delay(1_500L)
            when (
                val result = withContext(Dispatchers.IO) {
                    PairingClient.status(url, requestId)
                }
            ) {
                PairingClient.StatusResult.Pending -> Unit
                is PairingClient.StatusResult.Approved -> {
                    // Status returns the token once, so persist it before opening the socket.
                    store.putToken(hostKey, result.token)
                    connect(isAuthRetry = automatic)
                    return
                }
                PairingClient.StatusResult.Denied -> {
                    store.clearToken(hostKey)
                    showConnect("El PC rechazó la solicitud. Toca Conectar para reintentar")
                    return
                }
                PairingClient.StatusResult.Expired -> {
                    store.clearToken(hostKey)
                    showConnect("La solicitud expiró. Toca Conectar para reintentar")
                    return
                }
                PairingClient.StatusResult.Failed -> {
                    showConnect("No se pudo consultar la aprobación. Toca Conectar para reintentar")
                    return
                }
            }
        }
    }

    private fun cancelPairing() {
        pairingJob?.cancel()
        pairingJob = null
        showConnect("Solicitud cancelada. Toca Conectar para crear una nueva")
    }

    private fun hidePairingControls() {
        connectButton.visibility = View.GONE
        cancelPairingButton.visibility = View.GONE
    }

    private fun showConnect(message: String) {
        connectButton.visibility = View.VISIBLE
        cancelPairingButton.visibility = View.GONE
        setStatus(message)
    }

    private fun setStatus(text: String) {
        statusText.text = text
    }

    private fun setClipboardStatus(text: String) {
        runOnUiThread {
            if (!isFinishing && !isDestroyed) clipboardStatusText.text = text
        }
    }

    /** MediaStore writes on API 24-28 need WRITE_EXTERNAL_STORAGE; 29+ uses scoped storage. */
    private fun maybeRequestStoragePermission() {
        if (Build.VERSION.SDK_INT >= 29) return
        if (checkSelfPermission(android.Manifest.permission.WRITE_EXTERNAL_STORAGE)
            != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(android.Manifest.permission.WRITE_EXTERNAL_STORAGE), 1)
        }
    }

    private fun startClipListener() {
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val listener = ClipboardManager.OnPrimaryClipChangedListener {
            // A copy on the device syncs immediately instead of waiting for the next poll.
            lifecycleScope.launch(Dispatchers.IO) { runSync() }
        }
        clipListener = listener
        cm.addPrimaryClipChangedListener(listener)
    }

    private fun startSyncLoop() {
        lifecycleScope.launch(Dispatchers.IO) {
            while (isActive) {
                runSync()
                delay(POLL_MS)
            }
        }
    }

    /**
     * One sync step against the selected host. Local changes win over remote
     * ones; lastSyncedItem prevents content from echoing back to its source.
     */
    private fun runSync() {
        try {
            val local = readLocalClip()
            if (local != null && local != lastSyncedItem) {
                if (local.isImage && !shareImagesEnabled) {
                    // Do not let remote content overwrite an unsent local image.
                    lastSyncedItem = local
                    setClipboardStatus("Imagen local no compartida")
                    return
                }
                if (!push(serverUrl, local)) {
                    // Retry on the next poll because the server did not accept it.
                    Log.e(TAG, "push failed; item not marked synced")
                    setClipboardStatus("Error al enviar el portapapeles")
                    return
                }
                lastSyncedItem = local
                setClipboardStatus("Portapapeles sincronizado")
                return
            }
            val remote = fetchRemoteClip(serverUrl)
            if (remote != null && remote != lastSyncedItem) {
                if (local?.isImage == true && !shareImagesEnabled) return
                applyRemote(remote)
                lastSyncedItem = remote
                setClipboardStatus("Portapapeles sincronizado")
            } else {
                setClipboardStatus("Sincronización activa")
            }
        } catch (e: Exception) {
            Log.e(TAG, "Sync error: ${e.message}")
            setClipboardStatus("Sin conexión con el host")
        }
    }

    private fun readLocalClip(): ClipItem? {
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val clip = cm.primaryClip ?: return null
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
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        if (item.isImage) {
            val uri = saveImageToMediaStore(item.png ?: ByteArray(0)) ?: return
            cm.setPrimaryClip(ClipData.newUri(contentResolver, "shared", uri))
        } else {
            cm.setPrimaryClip(ClipData.newPlainText("shared", item.text ?: ""))
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
