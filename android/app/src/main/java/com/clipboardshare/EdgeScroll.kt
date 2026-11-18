package com.clipboardshare

/** The touchpad's right-edge strip: one finger dragged there scrolls instead of moving the pointer. */
object EdgeScroll {
    fun inStrip(x: Float, width: Int, stripPx: Float): Boolean =
        width > stripPx && x >= width - stripPx
}
