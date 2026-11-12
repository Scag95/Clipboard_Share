package com.clipboardshare

import android.content.ClipData
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.content.ClipboardManager
import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Bundle
import android.util.Log
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

class MainActivity : AppCompatActivity() {

    private val TAG = "ClipboardShare"
    private val SERVICE_TYPE = "_clipboard-share._tcp."
    private val client = OkHttpClient()

    private lateinit var nsdManager: NsdManager
    private lateinit var discoveryListener: NsdManager.DiscoveryListener
    private var serviceUrl: String? = null

    private lateinit var statusText: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        statusText = findViewById(R.id.statusText)
        nsdManager = getSystemService(Context.NSD_SERVICE) as NsdManager
        startDiscovery()
    }

    private fun startDiscovery() {
        discoveryListener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(regType: String) {
                Log.d(TAG, "Discovery started: $regType")
            }

            override fun onServiceFound(service: NsdServiceInfo) {
                if (service.serviceType == SERVICE_TYPE) {
                    nsdManager.resolveService(service, object : NsdManager.ResolveListener {
                        override fun onResolveFailed(si: NsdServiceInfo, errorCode: Int) {
                            Log.e(TAG, "Resolve failed: $errorCode")
                        }

                        override fun onServiceResolved(si: NsdServiceInfo) {
                            val host = si.host?.hostAddress ?: return
                            val port = si.port
                            serviceUrl = "http://$host:$port"
                            Log.d(TAG, "Resolved: $serviceUrl")
                            runOnUiThread { statusText.text = "Conectado a $serviceUrl" }
                            syncText()
                        }
                    })
                }
            }

            override fun onServiceLost(service: NsdServiceInfo) {
                Log.d(TAG, "Service lost: ${service.serviceName}")
                serviceUrl = null
                runOnUiThread { statusText.text = "Desconectado" }
            }

            override fun onDiscoveryStopped(serviceType: String) {
                Log.d(TAG, "Discovery stopped: $serviceType")
            }

            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                Log.e(TAG, "Start discovery failed: $errorCode")
            }

            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {
                Log.e(TAG, "Stop discovery failed: $errorCode")
            }
        }
        nsdManager.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, discoveryListener)
    }

    private fun syncText() {
        val url = serviceUrl ?: return
        lifecycleScope.launch {
            try {
                val remoteText = withContext(Dispatchers.IO) { getText(url) }
                val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                val clip = cm.primaryClip
                val localText = clip?.getItemAt(0)?.text?.toString()
                if (remoteText != null && remoteText != localText) {
                    cm.setPrimaryClip(ClipData.newPlainText("shared", remoteText))
                }
                if (localText != null && localText != remoteText) {
                    withContext(Dispatchers.IO) { setText(url, localText) }
                }
            } catch (e: Exception) {
                Log.e(TAG, "Sync error: ${e.message}")
            }
        }
    }

    private fun getText(url: String): String? {
        val req = Request.Builder().url("$url/clipboard/text").get().build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return null
            val body = resp.body?.string() ?: return null
            return JSONObject(body).optString("text", "")
        }
    }

    private fun setText(url: String, text: String) {
        val json = JSONObject().put("text", text).toString()
        val body = json.toRequestBody("application/json".toMediaType())
        val req = Request.Builder().url("$url/clipboard/text").post(body).build()
        client.newCall(req).execute().close()
    }


    private fun syncImage() {
        val url = serviceUrl ?: return
        lifecycleScope.launch {
            try {
                val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                val clip = cm.primaryClip
                val hasImage = clip != null && clip.description.hasMimeType("image/*")
                if (!hasImage) {
                    val bytes = withContext(Dispatchers.IO) { getImage(url) }
                    if (bytes != null) {
                        val bmp = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                        if (bmp != null) {
                            val values = android.content.ContentValues().apply {
                                put(android.provider.MediaStore.Images.Media.MIME_TYPE, "image/png")
                            }
                            val uri = contentResolver.insert(android.provider.MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values)
                            if (uri != null) {
                                contentResolver.openOutputStream(uri)?.use { out -> out.write(bytes) }
                                cm.setPrimaryClip(ClipData.newUri(contentResolver, "shared", uri))
                            }
                        }
                    }
                } else {
                    val uri = clip?.getItemAt(0)?.uri
                    val bmp = uri?.let { contentResolver.openInputStream(it)?.use { s -> BitmapFactory.decodeStream(s) } }
                    if (bmp != null) {
                        val out = java.io.ByteArrayOutputStream()
                        bmp.compress(Bitmap.CompressFormat.PNG, 100, out)
                        withContext(Dispatchers.IO) { setImage(url, out.toByteArray()) }
                    }
                }
            } catch (e: Exception) {
                Log.e(TAG, "Image sync error: ${e.message}")
            }
        }
    }

    private fun getImage(url: String): ByteArray? {
        val req = Request.Builder().url("$url/clipboard/image").get().build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return null
            return resp.body?.bytes()
        }
    }

    private fun setImage(url: String, data: ByteArray) {
        val body = data.toRequestBody("image/png".toMediaType())
        val req = Request.Builder().url("$url/clipboard/image").post(body).build()
        client.newCall(req).execute().close()
    }

    override fun onDestroy() {
        super.onDestroy()
        try {
            nsdManager.stopServiceDiscovery(discoveryListener)
        } catch (_: Exception) { }
    }
}
EOF