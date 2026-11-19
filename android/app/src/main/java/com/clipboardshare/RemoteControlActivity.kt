package com.clipboardshare

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.graphics.Color
import android.graphics.Typeface
import android.text.SpannableString
import android.text.style.ForegroundColorSpan
import android.text.style.StyleSpan
import android.text.style.UnderlineSpan
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.View
import android.widget.Button
import android.widget.CheckBox
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Remote mouse, keyboard, and clipboard for the host selected on the home screen.
 * Approved per-host tokens reconnect without user interaction.
 */
class RemoteControlActivity : AppCompatActivity() {

    private companion object {
        const val TAG = "ClipboardShareRemote"
        const val TYPED_CARD_VISIBLE_MS = 3_000L
    }

    private val client = InputClient()
    private val handler = Handler(Looper.getMainLooper())
    private val clipboardStatusHandler = Handler(Looper.getMainLooper())
    private lateinit var store: SessionStore
    private lateinit var serverUrl: String
    private lateinit var hostKey: String
    @Volatile private var manualDisconnect = false
    private var pairingJob: Job? = null

    private lateinit var statusText: TextView
    private lateinit var clipboardStatusText: TextView
    private lateinit var keyboard: RemoteKeyboardView
    private lateinit var statusBadge: Button
    private lateinit var statusDot: View
    private lateinit var connPanel: View
    private lateinit var connectButton: Button
    private lateinit var cancelPairingButton: Button
    private var retryScheduled = false
    private var authRetryInFlight = false

    private lateinit var shareImages: CheckBox
    private val clipPrefs by lazy { getSharedPreferences("clip_settings", Context.MODE_PRIVATE) }
    private val clipboardStatusPoller = object : Runnable {
        override fun run() {
            if (::clipboardStatusText.isInitialized) {
                val clip = ClipboardSyncService.clipboardStatus
                clipboardStatusText.text = clip
                statusBadge.text = "⎘ $clip ▾"
                statusDot.alpha = if (client.connected) 1f else 0.35f
            }
            clipboardStatusHandler.postDelayed(this, 500L)
        }
    }

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
        ClipboardSyncService.ensureStarted(this)

        setContentView(R.layout.activity_remote_control)
        statusText = findViewById(R.id.rcStatus)
        clipboardStatusText = findViewById(R.id.clipboardStatus)
        statusBadge = findViewById(R.id.btnStatusBadge)
        statusDot = findViewById(R.id.rcDot)
        connPanel = findViewById(R.id.connPanel)
        findViewById<TextView>(R.id.rcHost).text =
            store.getLastHosts().firstOrNull { SessionStore.hostKey(it.host, it.port) == hostKey }?.name
                ?: hostKey
        statusBadge.setOnClickListener {
            connPanel.visibility = if (connPanel.visibility == View.VISIBLE) View.GONE else View.VISIBLE
        }
        val moreShortcuts = findViewById<View>(R.id.moreShortcuts)
        val moreButton = findViewById<Button>(R.id.btnMoreShortcuts)
        moreButton.setOnClickListener {
            val open = moreShortcuts.visibility != View.VISIBLE
            moreShortcuts.visibility = if (open) View.VISIBLE else View.GONE
            moreButton.text = if (open) "▾" else "▴"
        }
        keyboard = findViewById(R.id.remoteKeyboard)
        keyboard.sink = object : KeyboardTranslator.Sink {
            override fun text(chars: String) = sendOrWarn(client.sendText(chars))
            override fun key(name: String) = sendOrWarn(client.sendKey(name))
        }
        val typedCard = findViewById<TextView>(R.id.typedCard)
        keyboard.onPhraseChanged = { phrase -> showTypedPhrase(typedCard, phrase) }
        watchSoftKeyboard()
        findViewById<Button>(R.id.btnKeyboard).setOnClickListener { keyboard.show() }
        connectButton = findViewById(R.id.btnConnect)
        cancelPairingButton = findViewById(R.id.btnCancelPairing)

        shareImages = findViewById(R.id.cbShareImages)
        shareImages.isChecked = clipPrefs.getBoolean("share_images", false)
        shareImages.setOnCheckedChangeListener { _, checked ->
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
        bind(R.id.btnTab) { client.sendKey("tab") }
        bind(R.id.btnDelete) { client.sendKey("delete") }
        bind(R.id.btnArrowLeft) { client.sendKey("arrowleft") }
        bind(R.id.btnArrowRight) { client.sendKey("arrowright") }
        bind(R.id.btnArrowUp) { client.sendKey("arrowup") }
        bind(R.id.btnArrowDown) { client.sendKey("arrowdown") }

        connectButton.setOnClickListener { requestPairing(automatic = false) }
        cancelPairingButton.setOnClickListener { cancelPairing() }
        bind(R.id.btnDisconnect) {
            connPanel.visibility = View.GONE
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
    }

    override fun onResume() {
        super.onResume()
        clipboardStatusHandler.removeCallbacks(clipboardStatusPoller)
        clipboardStatusHandler.post(clipboardStatusPoller)
        if (!::store.isInitialized || client.connected || pairingJob?.isActive == true) return
        manualDisconnect = false
        if (store.getToken(hostKey) != null) {
            connect()
        } else {
            requestPairing(automatic = false)
        }
    }

    override fun onPause() {
        clipboardStatusHandler.removeCallbacks(clipboardStatusPoller)
        super.onPause()
    }

    override fun onStart() {
        super.onStart()
        ClipboardSyncService.onActivityStarted()
    }

    override fun onStop() {
        ClipboardSyncService.onActivityStopped()
        super.onStop()
    }

    override fun onDestroy() {
        manualDisconnect = true
        pairingJob?.cancel()
        handler.removeCallbacksAndMessages(null)
        clipboardStatusHandler.removeCallbacksAndMessages(null)
        client.disconnect()
        super.onDestroy()
    }

    /** Latest words typed, with the one still being typed emphasised; it hides after a pause. */
    private fun showTypedPhrase(card: TextView, phrase: String) {
        card.removeCallbacks(hideTypedCard)
        if (phrase.isEmpty()) {
            card.visibility = View.GONE
            return
        }
        val (finished, current) = PhraseView.split(phrase)
        val text = SpannableString(finished + current)
        if (current.isNotEmpty()) {
            val start = finished.length
            text.setSpan(ForegroundColorSpan(Color.rgb(245, 245, 247)), start, text.length, 0)
            text.setSpan(StyleSpan(Typeface.BOLD), start, text.length, 0)
            text.setSpan(UnderlineSpan(), start, text.length, 0)
        }
        card.text = text
        card.visibility = View.VISIBLE
        typedCardView = card
        card.postDelayed(hideTypedCard, TYPED_CARD_VISIBLE_MS)
    }

    private var typedCardView: View? = null
    /** Dismissing the card also forgets the phrase, so the next one starts empty. */
    private val hideTypedCard = Runnable {
        typedCardView?.visibility = View.GONE
        keyboard.resetTyping()
    }

    /**
     * With the soft keyboard open only the keyboard button gives way; the shortcuts
     * stay usable and the readout floats above them, just over the keys.
     */
    private fun watchSoftKeyboard() {
        val root = findViewById<View>(android.R.id.content)
        val shortcuts = findViewById<View>(R.id.shortcutsPanel)
        val keyboardButton = findViewById<View>(R.id.btnKeyboard)
        val card = findViewById<View>(R.id.typedCard)
        val screenPadding = (14 * resources.displayMetrics.density).toInt()
        val gap = (8 * resources.displayMetrics.density).toInt()
        root.viewTreeObserver.addOnGlobalLayoutListener {
            val frame = android.graphics.Rect()
            root.getWindowVisibleDisplayFrame(frame)
            val hidden = root.rootView.height - frame.bottom
            val open = hidden > root.rootView.height * 0.15f
            val visibility = if (open) View.GONE else View.VISIBLE
            if (keyboardButton.visibility != visibility) keyboardButton.visibility = visibility
            // The card clears the shortcuts panel that sits above the keys.
            val margin = if (open) screenPadding + shortcuts.height + gap + gap else screenPadding
            val params = card.layoutParams as android.widget.FrameLayout.LayoutParams
            if (params.bottomMargin != margin) {
                params.bottomMargin = margin
                card.layoutParams = params
            }
            if (!open && typedCardView?.visibility == View.VISIBLE) {
                typedCardView?.removeCallbacks(hideTypedCard)
                hideTypedCard.run()
            }
        }
    }

    private fun sendOrWarn(sent: Boolean): Boolean {
        if (!sent) setStatus(if (client.connected) "No se pudo enviar la tecla" else "Sin conexión: la tecla no se envió")
        return sent
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
                    // The clipboard sync idles without a token: start it now that there is one.
                    ClipboardSyncService.ensureStarted(this@RemoteControlActivity)
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
                    // The clipboard sync idles without a token: start it now that there is one.
                    ClipboardSyncService.ensureStarted(this@RemoteControlActivity)
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

    /** MediaStore writes on API 24-28 need WRITE_EXTERNAL_STORAGE; 29+ uses scoped storage. */
    private fun maybeRequestStoragePermission() {
        if (Build.VERSION.SDK_INT >= 29) return
        if (checkSelfPermission(android.Manifest.permission.WRITE_EXTERNAL_STORAGE)
            != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(android.Manifest.permission.WRITE_EXTERNAL_STORAGE), 1)
        }
    }

}
