package com.clipboardshare

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class EdgeScrollTest {
    @Test fun touchInsideTheRightStripScrolls() {
        assertTrue(EdgeScroll.inStrip(x = 1000f, width = 1080, stripPx = 120f))
        assertTrue(EdgeScroll.inStrip(x = 960f, width = 1080, stripPx = 120f))
    }

    @Test fun touchLeftOfTheStripMovesThePointerAsBefore() {
        assertFalse(EdgeScroll.inStrip(x = 959f, width = 1080, stripPx = 120f))
        assertFalse(EdgeScroll.inStrip(x = 10f, width = 1080, stripPx = 120f))
    }

    @Test fun aViewNarrowerThanTheStripNeverScrolls() {
        assertFalse(EdgeScroll.inStrip(x = 50f, width = 100, stripPx = 120f))
    }
}
