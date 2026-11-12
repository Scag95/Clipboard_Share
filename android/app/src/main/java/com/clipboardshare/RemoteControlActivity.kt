package com.clipboardshare

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Remote mouse + keyboard control (S2, S5). Pairs with the server using its
 * printed PIN, keeps the session token, and streams events over WebSocket.
 * "Soltar" disconnects the input stream immediately (emergency hand back).
 */
class RemoteControlActivity : AppCompatActivity() {

    private val client = InputClient()
    private val handler = Handler(Looper.getMainLooper())
    private lateinit var store: SessionStore
    @Volatile private var manualDisconnect = false
    private var pendingPairConfirmUrl: String? = null

    private lateinit var statusText: TextView
    private lateinit var pinInput: EditText
    private lateinit var textInput: EditText
    private var retryScheduled = false

    private val wsListener = object : InputClient.Listener {
        override fun onConnected() {
            runOnUiThread { setStatus("Conectado a ${ServerRepo.url}") }
        }

        override fun onDisconnected(reason: String?) {
            runOnUiThread {
                if (reason == "unauthorized") {
                    store.token = null
                    pendingPairConfirmUrl = null
                    setStatus("Sesión expirada: ingresá de nuevo el PIN del server")
                } else if (manualDisconnect) {
                    setStatus("Desconectado (a mano)")
                } else {
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
        setContentView(R.layout.activity_remote_control)
        store = SessionStore(this)

        statusText = findViewById(R.id.rcStatus)
        pinInput = findViewById(R.id.etPin)
        textInput = findViewById(R.id.etText)

        findViewById<com.clipboardshare.TouchpadView>(R.id.touchpad).listener =
            object : com.clipboardshare.TouchpadView.Listener {
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

        bind(R.id.btnPair) { startPairing() }
        bind(R.id.btnConnect) { connect() }
        bind(R.id.btnDisconnect) {
            manualDisconnect = true
            handler.removeCallbacksAndMessages(null)
            retryScheduled = false
            client.disconnect()
            setStatus("Desconectado (a mano)")
        }
    }

    override fun onResume() {
        super.onResume()
        if (!client.connected) connect()
    }

    override fun onDestroy() {
        super.onDestroy()
        handler.removeCallbacksAndMessages(null)
        client.disconnect()
    }

    private fun bind(id: Int, action: () -> Unit) {
        findViewById<Button>(id).setOnClickListener { action() }
    }

    private fun connect() {
        manualDisconnect = false
        val token = store.token
        if (token == null) {
            setStatus("Ingresá el PIN que muestra el server y tocá Emparejar")
            return
        }
        val url = ServerRepo.url
        if (url == null) {
            setStatus("Buscando server...")
            scheduleRetry()
            return
        }
        if (client.connected) return
        setStatus("Conectando...")
        client.connect(url, token, wsListener)
    }

    private fun scheduleRetry() {
        if (retryScheduled) return
        retryScheduled = true
        handler.postDelayed(
            {
                retryScheduled = false
                if (!client.connected && !isFinishing) connect()
            },
            2_500L,
        )
    }

    private fun startPairing() {
        val url = ServerRepo.url
        if (url == null) {
            setStatus("Server no encontrado en la red todavía")
            return
        }
        val pin = pinInput.text.toString().trim()
        if (!Regex("^\\d{6}$").matches(pin)) {
            setStatus("El PIN son 6 dígitos")
            return
        }
        // HTTP is unauthenticated on the LAN: show which host receives the PIN
        // and require a second explicit tap before sending it.
        if (pendingPairConfirmUrl != url) {
            pendingPairConfirmUrl = url
            setStatus("Confirmá: tocá Emparejar de nuevo para enviar el PIN a $url")
            return
        }
        pendingPairConfirmUrl = null
        setStatus("Emparejando...")
        lifecycleScope.launch(Dispatchers.IO) {
            val token = PairingClient.pair(url, pin)
            withContext(Dispatchers.Main) {
                if (token != null) {
                    store.token = token
                    pinInput.setText("")
                    setStatus("Emparejado")
                    connect()
                } else {
                    setStatus("PIN inválido o server inaccesible")
                }
            }
        }
    }

    private fun setStatus(text: String) {
        statusText.text = text
    }
}
