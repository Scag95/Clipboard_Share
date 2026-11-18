package com.clipboardshare

import android.Manifest
import android.animation.ObjectAnimator
import android.animation.ValueAnimator
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.format.DateUtils
import android.util.Log
import android.view.Gravity
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

/** Home inventory: every live NSD service plus previously selected hosts. */
class MainActivity : AppCompatActivity() {

    private companion object {
        const val TAG = "ClipboardShare"
        const val SERVICE_TYPE = "_clipboard-share._tcp."
        const val NOTIFICATION_PERMISSION_REQUEST = 1001
        const val SEARCH_WINDOW_MS = 4_000L
    }

    private lateinit var nsdManager: NsdManager
    private lateinit var discoveryListener: NsdManager.DiscoveryListener
    private lateinit var store: SessionStore
    private lateinit var hostList: LinearLayout
    private lateinit var statusText: TextView
    private lateinit var scanDot: View
    private lateinit var rescanButton: Button
    private val uiHandler = Handler(Looper.getMainLooper())
    private var scanAnimator: ObjectAnimator? = null
    private var searching = true
    private var discoveryError = false
    private var liveCount = 0

    /** Services are aggregated by name; resolving one never replaces another. */
    private val discoveredServices = linkedMapOf<String, NsdServiceInfo>()
    private val resolvingServices = mutableSetOf<String>()
    private val probingHosts = mutableSetOf<String>()

    private val probeClient = OkHttpClient.Builder()
        .connectTimeout(1_500, TimeUnit.MILLISECONDS)
        .readTimeout(1_500, TimeUnit.MILLISECONDS)
        .writeTimeout(1_500, TimeUnit.MILLISECONDS)
        .callTimeout(1_500, TimeUnit.MILLISECONDS)
        .build()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        // The layout carries its own title; the themed action bar would repeat it.
        supportActionBar?.hide()
        store = SessionStore(this)
        hostList = findViewById(R.id.hostList)
        statusText = findViewById(R.id.statusText)
        scanDot = findViewById(R.id.scanDot)
        rescanButton = findViewById(R.id.btnRescan)
        rescanButton.setOnClickListener { rescan() }
        nsdManager = getSystemService(Context.NSD_SERVICE) as NsdManager
        if (store.selectedHost != null) startClipboardSync()
        renderHosts()
        startDiscovery()
        beginSearchState()
    }

    /** Restarts NSD discovery from scratch; remembered hosts stay in the list. */
    private fun rescan() {
        try {
            nsdManager.stopServiceDiscovery(discoveryListener)
        } catch (_: Exception) {
        }
        discoveredServices.clear()
        resolvingServices.clear()
        renderHosts()
        startDiscovery()
        beginSearchState()
    }

    private fun beginSearchState() {
        searching = true
        discoveryError = false
        uiHandler.removeCallbacks(endSearchRunnable)
        uiHandler.postDelayed(endSearchRunnable, SEARCH_WINDOW_MS)
        updateStatus()
    }

    private val endSearchRunnable = Runnable {
        searching = false
        updateStatus()
    }

    private fun updateStatus() {
        statusText.text = when {
            discoveryError -> "No se pudo buscar dispositivos en la red"
            searching -> "Buscando dispositivos en tu red…"
            liveCount == 0 -> "Ningún dispositivo en red"
            liveCount == 1 -> "1 dispositivo en red"
            else -> "$liveCount dispositivos en red"
        }
        rescanButton.isEnabled = !searching
        rescanButton.text = if (searching) "Buscando…" else "↻ Buscar de nuevo"
        rescanButton.setTextColor(if (searching) Color.rgb(168, 168, 179) else Color.rgb(245, 245, 247))
        if (searching && scanAnimator == null) {
            scanAnimator = ObjectAnimator.ofFloat(scanDot, View.ALPHA, 1f, 0.25f).apply {
                duration = 800
                repeatCount = ValueAnimator.INFINITE
                repeatMode = ValueAnimator.REVERSE
                start()
            }
        } else if (!searching) {
            scanAnimator?.cancel()
            scanAnimator = null
            scanDot.alpha = if (discoveryError) 0.35f else 0.6f
        }
    }

    private fun startDiscovery() {
        discoveryListener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(regType: String) {
                Log.d(TAG, "Discovery started: $regType")
            }

            override fun onServiceFound(service: NsdServiceInfo) {
                if (service.serviceType.removeSuffix(".") != SERVICE_TYPE.removeSuffix(".")) return
                runOnUiThread {
                    if (!resolvingServices.add(service.serviceName)) return@runOnUiThread
                    nsdManager.resolveService(service, object : NsdManager.ResolveListener {
                        override fun onResolveFailed(si: NsdServiceInfo, errorCode: Int) {
                            Log.e(TAG, "Resolve failed for ${si.serviceName}: $errorCode")
                            runOnUiThread { resolvingServices.remove(si.serviceName) }
                        }

                        override fun onServiceResolved(si: NsdServiceInfo) {
                            Log.d(TAG, "Resolved: ${si.serviceName} ${si.host}:${si.port} all=${if (Build.VERSION.SDK_INT >= 34) si.hostAddresses else null}")
                            runOnUiThread {
                                resolvingServices.remove(si.serviceName)
                                if (si.port > 0) {
                                    discoveredServices[si.serviceName] = si
                                    renderHosts()
                                }
                            }
                        }
                    })
                }
            }

            override fun onServiceLost(service: NsdServiceInfo) {
                Log.d(TAG, "Service lost: ${service.serviceName}")
                runOnUiThread {
                    resolvingServices.remove(service.serviceName)
                    discoveredServices.remove(service.serviceName)
                    renderHosts()
                }
            }

            override fun onDiscoveryStopped(serviceType: String) {
                Log.d(TAG, "Discovery stopped: $serviceType")
            }

            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                Log.e(TAG, "Start discovery failed: $errorCode")
                runOnUiThread {
                    discoveryError = true
                    searching = false
                    updateStatus()
                }
            }

            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {
                Log.e(TAG, "Stop discovery failed: $errorCode")
            }
        }
        try {
            nsdManager.discoverServices(
                SERVICE_TYPE,
                NsdManager.PROTOCOL_DNS_SD,
                discoveryListener,
            )
        } catch (e: Exception) {
            Log.e(TAG, "Cannot start discovery", e)
            discoveryError = true
            searching = false
            updateStatus()
        }
    }

    private fun renderHosts() {
        val rememberedAll = store.getLastHosts()
        val live = discoveredServices.mapNotNull { (name, service) ->
            // NSD sometimes resolves only a link-local address; the same service
            // is still on the network, so fall back to the address remembered for it.
            val host = pickAddress(service)
                ?: rememberedAll.firstOrNull { it.name == name && it.port == service.port }?.host
                ?: run {
                    Log.w(TAG, "No usable address for $name")
                    return@mapNotNull null
                }
            SessionStore.LastHost(name, host, service.port, 0L)
        }.sortedBy { it.name.lowercase() }
        val liveKeys = live.mapTo(mutableSetOf()) { SessionStore.hostKey(it.host, it.port) }
        val liveNames = live.mapTo(mutableSetOf()) { it.name }
        val remembered = rememberedAll
            .filter { SessionStore.hostKey(it.host, it.port) !in liveKeys && it.name !in liveNames }
            .sortedByDescending { it.lastConnected }

        hostList.removeAllViews()
        liveCount = live.size
        if (live.isNotEmpty()) {
            hostList.addView(sectionHeader("En tu red"))
            live.forEach { host -> hostList.addView(createHostRow(host, live = true)) }
        }
        if (remembered.isNotEmpty()) {
            hostList.addView(sectionHeader("Recordados"))
            remembered.forEach { host -> hostList.addView(createHostRow(host, live = false)) }
        }
        if (::rescanButton.isInitialized) updateStatus()
    }

    private fun sectionHeader(title: String): View = TextView(this).apply {
        text = title.uppercase()
        textSize = 12f
        letterSpacing = 0.1f
        setTextColor(Color.rgb(168, 168, 179))
        setPadding(dp(2), dp(6), 0, dp(8))
    }

    private fun createHostRow(host: SessionStore.LastHost, live: Boolean): View {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(16), dp(16), dp(16), dp(16))
            setBackgroundResource(R.drawable.bg_card)
            alpha = if (live) 1f else 0.8f
            isClickable = true
            isFocusable = true
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT,
            ).apply { bottomMargin = dp(10) }
        }
        row.addView(
            TextView(this).apply {
                text = if (live) "🖥" else "💻"
                textSize = 22f
                gravity = Gravity.CENTER
                setBackgroundResource(R.drawable.bg_icon_box)
                layoutParams = LinearLayout.LayoutParams(dp(44), dp(44)).apply { marginEnd = dp(14) }
                importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO
            }
        )
        val detail = if (live || host.lastConnected <= 0L) {
            host.host
        } else {
            "Visto " + DateUtils.getRelativeTimeSpanString(
                host.lastConnected, System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS,
            ).toString().lowercase() + " · toca para reintentar"
        }
        row.addView(
            LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL
                layoutParams = LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f)
                addView(
                    TextView(context).apply {
                        text = host.name
                        textSize = 17f
                        setTextColor(Color.rgb(245, 245, 247))
                        setTypeface(typeface, android.graphics.Typeface.BOLD)
                    }
                )
                addView(
                    TextView(context).apply {
                        text = detail
                        textSize = 13f
                        setTextColor(Color.rgb(168, 168, 179))
                    }
                )
            }
        )
        row.addView(
            TextView(this).apply {
                text = if (live) "En red" else "Sin respuesta"
                textSize = 12.5f
                setTextColor(if (live) Color.rgb(91, 214, 138) else Color.rgb(168, 168, 179))
                setBackgroundResource(if (live) R.drawable.bg_chip_on else R.drawable.bg_chip)
                setPadding(dp(10), dp(6), dp(10), dp(6))
            }
        )
        row.setOnClickListener {
            if (live) openRemote(host) else probeRememberedHost(host)
        }
        return row
    }

    private fun probeRememberedHost(host: SessionStore.LastHost) {
        val key = SessionStore.hostKey(host.host, host.port)
        if (!probingHosts.add(key)) return
        lifecycleScope.launch {
            val reachable = withContext(Dispatchers.IO) {
                try {
                    val request = Request.Builder().url("${serverUrl(host)}/health").get().build()
                    probeClient.newCall(request).execute().use { it.isSuccessful }
                } catch (e: Exception) {
                    Log.d(TAG, "Health probe failed for $key: ${e.message}")
                    false
                }
            }
            probingHosts.remove(key)
            if (reachable) {
                openRemote(host)
            } else {
                Toast.makeText(
                    this@MainActivity,
                    "${host.name} no responde. Verifica que esté encendido y vuelve a intentar.",
                    Toast.LENGTH_LONG,
                ).show()
            }
        }
    }

    private fun openRemote(host: SessionStore.LastHost) {
        val selected = host.copy(lastConnected = System.currentTimeMillis())
        store.putLastHost(selected)
        startClipboardSync()
        startActivity(
            Intent(this, RemoteControlActivity::class.java)
                .putExtra("server_url", serverUrl(selected))
        )
    }

    private fun startClipboardSync() {
        if (
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(
                arrayOf(Manifest.permission.POST_NOTIFICATIONS),
                NOTIFICATION_PERMISSION_REQUEST,
            )
        }
        ClipboardSyncService.ensureStarted(this)
    }

    private fun serverUrl(host: SessionStore.LastHost): String =
        HostAddress.serverUrl(host.host, host.port)

    @Suppress("DEPRECATION")
    private fun pickAddress(service: NsdServiceInfo): String? {
        val addresses = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            service.hostAddresses
        } else {
            listOfNotNull(service.host)
        }
        return HostAddress.pick(addresses)
    }

    private fun dp(value: Int): Int = (value * resources.displayMetrics.density).toInt()

    override fun onStart() {
        super.onStart()
        ClipboardSyncService.onActivityStarted()
    }

    override fun onStop() {
        ClipboardSyncService.onActivityStopped()
        super.onStop()
    }

    override fun onDestroy() {
        uiHandler.removeCallbacksAndMessages(null)
        scanAnimator?.cancel()
        try {
            nsdManager.stopServiceDiscovery(discoveryListener)
        } catch (_: Exception) {
        }
        super.onDestroy()
    }
}
