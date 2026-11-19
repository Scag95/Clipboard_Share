package com.clipboardshare

import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress

/**
 * Chooses a host address OkHttp can put in a URL. NSD often reports an IPv6
 * link-local address ("fe80::…%wlan0"); OkHttp rejects its zone id, so such an
 * address is never usable and IPv4 is preferred.
 */
object HostAddress {
    fun pick(addresses: List<InetAddress>): String? {
        addresses.firstOrNull { it is Inet4Address }?.hostAddress?.let { return it }
        return addresses.firstOrNull { it is Inet6Address && !it.isLinkLocalAddress }
            ?.hostAddress?.substringBefore('%')
    }

    /** False for a zoned or link-local IPv6 literal, which OkHttp cannot connect to. */
    fun isUsable(host: String): Boolean =
        host.isNotEmpty() && '%' !in host && !host.startsWith("fe80:", ignoreCase = true)

    /** Every app <-> server call is HTTPS (S6); the PC presents a pinned self-signed certificate. */
    fun serverUrl(host: String, port: Int): String {
        val address = if (host.contains(':') && !host.startsWith("[")) "[$host]" else host
        return "https://$address:$port"
    }

    /** The server URL of a `host:port` session key, or null when the key has no valid port. */
    fun serverUrlForHostKey(key: String): String? {
        val separator = key.lastIndexOf(':')
        if (separator <= 0 || separator == key.lastIndex) return null
        val host = key.substring(0, separator)
        val port = key.substring(separator + 1).toIntOrNull() ?: return null
        if (port !in 1..65535) return null
        return serverUrl(host, port)
    }

    /** The WebSocket (wss) URL of [path] on the server at [serverUrl], built by [serverUrl]. */
    fun webSocketUrl(serverUrl: String, path: String): String =
        "wss://" + serverUrl.substringAfter("://") + path
}
