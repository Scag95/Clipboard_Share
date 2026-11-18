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

    fun serverUrl(host: String, port: Int): String {
        val address = if (host.contains(':') && !host.startsWith("[")) "[$host]" else host
        return "http://$address:$port"
    }
}
