package com.clipboardshare

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.Gravity
import android.view.View
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
    }

    private lateinit var nsdManager: NsdManager
    private lateinit var discoveryListener: NsdManager.DiscoveryListener
    private lateinit var store: SessionStore
    private lateinit var hostList: LinearLayout
    private lateinit var statusText: TextView

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
        store = SessionStore(this)
        hostList = findViewById(R.id.hostList)
        statusText = findViewById(R.id.statusText)
        nsdManager = getSystemService(Context.NSD_SERVICE) as NsdManager
        if (store.selectedHost != null) startClipboardSync()
        renderHosts()
        startDiscovery()
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
                            Log.d(TAG, "Resolved: ${si.serviceName} ${si.host}:${si.port}")
                            runOnUiThread {
                                resolvingServices.remove(si.serviceName)
                                if (si.host?.hostAddress != null && si.port > 0) {
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
                    if (hostList.childCount == 0) {
                        statusText.visibility = View.VISIBLE
                        statusText.text = "No se pudo buscar dispositivos en la red"
                    }
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
            statusText.text = "No se pudo buscar dispositivos en la red"
        }
    }

    private fun renderHosts() {
        val live = discoveredServices.mapNotNull { (name, service) ->
            val host = service.host?.hostAddress ?: return@mapNotNull null
            SessionStore.LastHost(name, host, service.port, 0L)
        }.sortedBy { it.name.lowercase() }
        val liveKeys = live.mapTo(mutableSetOf()) { SessionStore.hostKey(it.host, it.port) }
        val liveNames = live.mapTo(mutableSetOf()) { it.name }
        val remembered = store.getLastHosts()
            .filter { SessionStore.hostKey(it.host, it.port) !in liveKeys && it.name !in liveNames }
            .sortedByDescending { it.lastConnected }

        hostList.removeAllViews()
        live.forEach { host -> hostList.addView(createHostRow(host, live = true)) }
        remembered.forEach { host -> hostList.addView(createHostRow(host, live = false)) }
        statusText.visibility = if (live.isEmpty() && remembered.isEmpty()) View.VISIBLE else View.GONE
        if (live.isEmpty() && remembered.isEmpty()) {
            statusText.text = "Buscando dispositivos…"
        }
    }

    private fun createHostRow(host: SessionStore.LastHost, live: Boolean): View {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(16), dp(14), dp(16), dp(14))
            setBackgroundResource(R.drawable.trackpad_surface)
            isClickable = true
            isFocusable = true
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT,
            ).apply { bottomMargin = dp(8) }
        }
        row.addView(
            TextView(this).apply {
                text = host.name
                textSize = 17f
                setTextColor(Color.rgb(245, 245, 247))
                layoutParams = LinearLayout.LayoutParams(
                    0,
                    LinearLayout.LayoutParams.WRAP_CONTENT,
                    1f,
                )
            }
        )
        row.addView(
            TextView(this).apply {
                text = if (live) "en red" else "sin respuesta"
                textSize = 14f
                setTextColor(if (live) Color.rgb(128, 203, 196) else Color.rgb(176, 176, 182))
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

    private fun serverUrl(host: SessionStore.LastHost): String {
        val address = if (host.host.contains(':') && !host.host.startsWith("[")) {
            "[${host.host}]"
        } else {
            host.host
        }
        return "http://$address:${host.port}"
    }

    private fun dp(value: Int): Int = (value * resources.displayMetrics.density).toInt()

    override fun onDestroy() {
        try {
            nsdManager.stopServiceDiscovery(discoveryListener)
        } catch (_: Exception) {
        }
        super.onDestroy()
    }
}
