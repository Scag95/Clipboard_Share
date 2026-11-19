package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.net.InetAddress

class HostAddressTest {
    private fun ip(literal: String) = InetAddress.getByName(literal)

    @Test
    fun prefersIpv4OverLinkLocalIpv6() {
        val picked = HostAddress.pick(listOf(ip("fe80::1"), ip("192.168.0.14")))
        assertEquals("192.168.0.14", picked)
    }

    @Test
    fun usesGlobalIpv6WhenThereIsNoIpv4() {
        assertEquals("2001:db8:0:0:0:0:0:5", HostAddress.pick(listOf(ip("fe80::1"), ip("2001:db8::5"))))
    }

    @Test
    fun rejectsLinkLocalOnlyBecauseOkHttpCannotParseItsZone() {
        assertNull(HostAddress.pick(listOf(ip("fe80::1"))))
    }

    @Test
    fun returnsNullForNoAddresses() {
        assertNull(HostAddress.pick(emptyList()))
    }

    @Test
    fun serverUrlBracketsIpv6AndLeavesIpv4Alone() {
        assertEquals("https://[2001:db8::5]:8765", HostAddress.serverUrl("2001:db8::5", 8765))
        assertEquals("https://192.168.0.14:8765", HostAddress.serverUrl("192.168.0.14", 8765))
    }

    @Test
    fun webSocketUrlIsWssOnTheSameHostAndPort() {
        assertEquals(
            "wss://192.168.0.14:8765/ws/input",
            HostAddress.webSocketUrl("https://192.168.0.14:8765", "/ws/input"),
        )
        assertEquals(
            "wss://[2001:db8::5]:8765/ws/input",
            HostAddress.webSocketUrl("https://[2001:db8::5]:8765", "/ws/input"),
        )
    }

    @Test
    fun serverUrlForHostKeyIsHttps() {
        assertEquals("https://192.168.0.14:8765", HostAddress.serverUrlForHostKey("192.168.0.14:8765"))
        assertEquals("https://[2001:db8::5]:8765", HostAddress.serverUrlForHostKey("2001:db8::5:8765"))
        assertEquals("https://[2001:db8::5]:8765", HostAddress.serverUrlForHostKey("[2001:db8::5]:8765"))
    }

    @Test
    fun serverUrlForHostKeyRejectsKeysWithoutAValidPort() {
        for (bad in listOf("", "host", ":8765", "host:", "host:0", "host:65536", "host:abc")) {
            assertNull(bad, HostAddress.serverUrlForHostKey(bad))
        }
    }

    @Test
    fun usableRejectsZonedAndLinkLocalAddressesButKeepsRoutableOnes() {
        assertEquals(true, HostAddress.isUsable("192.168.0.14"))
        assertEquals(true, HostAddress.isUsable("2001:db8::5"))
        assertEquals(false, HostAddress.isUsable("fe80::ea12:b8b6:7fae:ebbd%wlan0"))
        assertEquals(false, HostAddress.isUsable("fe80::1"))
        assertEquals(false, HostAddress.isUsable(""))
    }
}
