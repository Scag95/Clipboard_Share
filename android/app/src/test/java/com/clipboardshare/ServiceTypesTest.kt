package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ServiceTypesTest {
    @Test
    fun browsesOnlyNudge() {
        assertEquals(listOf("_nudge._tcp."), ServiceTypes.ALL)
    }

    @Test
    fun acceptsNudgeWithOrWithoutTrailingDot() {
        assertTrue(ServiceTypes.isKnown("_nudge._tcp."))
        assertTrue(ServiceTypes.isKnown("_nudge._tcp"))
    }

    @Test
    fun rejectsOtherTypes() {
        assertFalse(ServiceTypes.isKnown("_http._tcp."))
        assertFalse(ServiceTypes.isKnown(""))
        assertFalse(ServiceTypes.isKnown(null))
    }
}
