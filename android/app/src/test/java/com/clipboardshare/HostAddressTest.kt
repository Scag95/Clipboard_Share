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
        assertEquals("http://[2001:db8::5]:8765", HostAddress.serverUrl("2001:db8::5", 8765))
        assertEquals("http://192.168.0.14:8765", HostAddress.serverUrl("192.168.0.14", 8765))
    }
}
