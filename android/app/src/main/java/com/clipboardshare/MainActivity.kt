package com.clipboardshare

import android.content.ClipData
import android.content.ClipboardManager
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.os.Bundle
import android.provider.MediaStore
import android.util.Log
import android.widget.Button
import android.widget.CheckBox
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
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

class MainActivity : AppCompatActivity() {

    private companion object {
        const val TAG = "ClipboardShare"
        const val SERVICE_TYPE = "_clipboard-share._tcp."
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

    private val client = OkHttpClient.Builder()
        .connectTimeout(3, TimeUnit.SECONDS)
        .readTimeout(10, TimeUnit.SECONDS)
        .build()

    private lateinit var nsdManager: NsdManager
    private lateinit var discoveryListener: NsdManager.DiscoveryListener
    private lateinit var clipListener: ClipboardManager.OnPrimaryClipChangedListener
    @Volatile private var serviceUrl: String? = null

    /** Last content this device has already propagated in both directions. */
    @Volatile private var lastSyncedItem: ClipItem? = null

    private lateinit var statusText: TextView
    private lateinit var shareImages: CheckBox
    private val clipPrefs by lazy { getSharedPreferences("clip_settings", Context.MODE_PRIVATE) }

    /** Opt-in: local clipboard images are never pushed without this checkbox (R1). */
    @Volatile private var shareImagesEnabled = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        statusText = findViewById(R.id.statusText)
        findViewById<Button>(R.id.remoteButton).setOnClickListener {
            startActivity(Intent(this, RemoteControlActivity::class.java))
        }
        shareImages = findViewById(R.id.cbShareImages)
        shareImagesEnabled = clipPrefs.getBoolean("share_images", false)
        shareImages.isChecked = shareImagesEnabled
        shareImages.setOnCheckedChangeListener { _, checked ->
            shareImagesEnabled = checked
            clipPrefs.edit().putBoolean("share_images", checked).apply()
        }
        maybeRequestStoragePermission()
        nsdManager = getSystemService(Context.NSD_SERVICE) as NsdManager
        startDiscovery()
        startClipListener()
        startSyncLoop()
    }

    /** MediaStore writes on API 24-28 need WRITE_EXTERNAL_STORAGE; 29+ uses scoped storage. */
    private fun maybeRequestStoragePermission() {
        if (Build.VERSION.SDK_INT >= 29) return
        if (checkSelfPermission(android.Manifest.permission.WRITE_EXTERNAL_STORAGE)
            != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(
                arrayOf(android.Manifest.permission.WRITE_EXTERNAL_STORAGE), 1
            )
        }
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
                            ServerRepo.url = serviceUrl
                            Log.d(TAG, "Resolved: $serviceUrl")
                            runOnUiThread { statusText.text = "Conectado a $serviceUrl" }
                        }
                    })
                }
            }

            override fun onServiceLost(service: NsdServiceInfo) {
                Log.d(TAG, "Service lost: ${service.serviceName}")
                serviceUrl = null
                ServerRepo.url = null
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

    private fun startClipListener() {
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        clipListener = ClipboardManager.OnPrimaryClipChangedListener {
            // A copy on the device syncs immediately instead of waiting for the next poll.
            lifecycleScope.launch(Dispatchers.IO) { runSync() }
        }
        cm.addPrimaryClipChangedListener(clipListener)
    }

    private fun startSyncLoop() {
        lifecycleScope.launch(Dispatchers.IO) {
            while (isActive) {
                if (serviceUrl != null) runSync()
                delay(POLL_MS)
            }
        }
    }

    /**
     * One sync step. Local changes win over remote ones; once either side is
     * propagated, lastSyncedItem pins both, so the same content is never
     * echoed back to its source.
     */
    private fun runSync() {
        val url = serviceUrl ?: return
        try {
            val local = readLocalClip()
            if (local != null && local != lastSyncedItem) {
                if (local.isImage && !shareImagesEnabled) {
                    // Pin the content without sending it, and end this sync
                    // step: fetching remote content now could overwrite the
                    // user's unsent local image.
                    lastSyncedItem = local
                    return
                }
                push(url, local)
                lastSyncedItem = local
                return
            }
            val remote = fetchRemoteClip(url)
            if (remote != null && remote != lastSyncedItem) {
                if (local?.isImage == true && !shareImagesEnabled) {
                    // An unsent local image wins: applying remote text would
                    // silently overwrite content the user wanted kept.
                    return
                }
                applyRemote(remote)
                lastSyncedItem = remote
            }
        } catch (e: Exception) {
            Log.e(TAG, "Sync error: ${e.message}")
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

    private fun push(url: String, item: ClipItem) {
        if (item.isImage) {
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
            MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values
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
        client.newCall(req).execute().use { }
    }

    private fun getImage(url: String): ByteArray? {
        val req = Request.Builder().url("$url/clipboard/image").get().build()
        client.newCall(req).execute().use { resp ->
            if (resp.code == 204) return null
            if (!resp.isSuccessful) return null
            return resp.body?.bytes()
        }
    }

    private fun setImage(url: String, data: ByteArray) {
        val body = data.toRequestBody("image/png".toMediaType())
        val req = Request.Builder().url("$url/clipboard/image").post(body).build()
        client.newCall(req).execute().use { }
    }

    override fun onDestroy() {
        super.onDestroy()
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.removePrimaryClipChangedListener(clipListener)
        try {
            nsdManager.stopServiceDiscovery(discoveryListener)
        } catch (_: Exception) { }
    }
}
