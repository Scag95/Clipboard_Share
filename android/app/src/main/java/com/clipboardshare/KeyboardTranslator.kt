package com.clipboardshare

/**
 * Turns soft-keyboard operations into remote key events, as a physical
 * keyboard would. It keeps no text buffer: the cursor and selection live on
 * the PC. Only the keyboard's current composing string is remembered so a
 * rewrite can erase and retype just the part that changed.
 */
class KeyboardTranslator(private val sink: Sink) {
    interface Sink {
        fun text(chars: String): Boolean
        fun key(name: String): Boolean
    }

    private var composing = ""

    fun commit(text: CharSequence): Boolean {
        val ok = rewriteComposing(text.toString())
        composing = ""
        return ok
    }

    fun setComposing(text: CharSequence): Boolean {
        val ok = rewriteComposing(text.toString())
        composing = text.toString()
        return ok
    }

    fun finishComposing() {
        composing = ""
    }

    fun deleteBefore(count: Int): Boolean {
        composing = ""
        if (count <= 0 || count > MAX_BACKSPACES) return false
        return repeatBackspace(count)
    }

    fun pressKey(name: String): Boolean {
        composing = ""
        return sink.key(name)
    }

    /** Erases the tail of the old composition that differs, then types the new tail. */
    private fun rewriteComposing(next: String): Boolean {
        var common = 0
        val limit = minOf(composing.length, next.length)
        while (common < limit && composing[common] == next[common]) common++
        // Never split a surrogate pair at the boundary.
        if (common > 0 && Character.isHighSurrogate(next[common - 1])) common--
        val toErase = Character.codePointCount(composing, common, composing.length)
        var ok = true
        if (toErase > 0) ok = (toErase <= MAX_BACKSPACES) && repeatBackspace(toErase)
        return typeText(next.substring(common)) && ok
    }

    private fun repeatBackspace(n: Int): Boolean {
        var ok = true
        repeat(n) { ok = sink.key("backspace") && ok }
        return ok
    }

    private fun typeText(text: String): Boolean {
        var ok = true
        val run = StringBuilder()
        fun flush() {
            if (run.isEmpty()) return
            chunk(run.toString()).forEach { ok = sink.text(it) && ok }
            run.setLength(0)
        }
        for (ch in text) {
            when {
                ch == '\n' -> { flush(); ok = sink.key("enter") && ok }
                ch.code < 0x20 || ch.code == 0x7f -> Unit
                else -> run.append(ch)
            }
        }
        flush()
        return ok
    }

    private fun chunk(s: String): List<String> {
        val parts = mutableListOf<String>()
        var start = 0
        while (start < s.length) {
            val count = minOf(MAX_TEXT_CODE_POINTS, s.codePointCount(start, s.length))
            val end = s.offsetByCodePoints(start, count)
            parts += s.substring(start, end)
            start = end
        }
        return parts
    }

    companion object {
        const val MAX_TEXT_CODE_POINTS = 1000
        const val MAX_BACKSPACES = 100
    }
}
