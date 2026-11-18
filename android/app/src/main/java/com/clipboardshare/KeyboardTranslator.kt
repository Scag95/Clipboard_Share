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

    /** The word being typed, for on-phone feedback only; the PC owns the real text. */
    private val word = StringBuilder()
    val currentWord: String get() = word.toString()
    var onWordChanged: ((String) -> Unit)? = null
    private var lastReported = ""

    /** The most recent text typed, for the floating readout; cleared when the PC cursor may have moved. */
    private val phrase = StringBuilder()
    val currentPhrase: String get() = phrase.toString()
    var onPhraseChanged: ((String) -> Unit)? = null
    private var lastPhrase = ""

    private fun reportWord() {
        val now = word.toString()
        if (now != lastReported) {
            lastReported = now
            onWordChanged?.invoke(now)
        }
        val text = phrase.toString()
        if (text != lastPhrase) {
            lastPhrase = text
            onPhraseChanged?.invoke(text)
        }
    }

    private fun trimPhrase() {
        val extra = phrase.length - MAX_PHRASE
        if (extra <= 0) return
        var cut = extra
        if (Character.isLowSurrogate(phrase[cut])) cut++
        phrase.delete(0, cut)
    }

    fun commit(text: CharSequence): Boolean {
        val ok = rewriteComposing(text.toString())
        composing = ""
        reportWord()
        return ok
    }

    fun setComposing(text: CharSequence): Boolean {
        val ok = rewriteComposing(text.toString())
        composing = text.toString()
        reportWord()
        return ok
    }

    fun finishComposing() {
        composing = ""
    }

    fun deleteBefore(count: Int): Boolean {
        composing = ""
        if (count <= 0 || count > MAX_BACKSPACES) return false
        val ok = repeatBackspace(count)
        reportWord()
        return ok
    }

    fun pressKey(name: String): Boolean {
        // The keyboard's delete key arrives here as a key event; it must still shorten the readout.
        if (name == "backspace") return deleteBefore(1)
        composing = ""
        word.setLength(0)
        if (name == "enter") {
            phrase.append(ENTER_MARK)
            trimPhrase()
        } else {
            phrase.setLength(0)
        }
        val ok = sink.key(name)
        reportWord()
        return ok
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
        repeat(n) {
            ok = sink.key("backspace") && ok
            if (word.isNotEmpty()) word.setLength(word.offsetByCodePoints(word.length, -1))
            if (phrase.isNotEmpty()) phrase.setLength(phrase.offsetByCodePoints(phrase.length, -1))
        }
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
                ch == '\n' -> { flush(); word.setLength(0); phrase.append(ENTER_MARK); ok = sink.key("enter") && ok }
                ch.code < 0x20 || ch.code == 0x7f -> Unit
                else -> {
                    run.append(ch)
                    phrase.append(ch)
                    if (Character.isWhitespace(ch)) word.setLength(0) else word.append(ch)
                }
            }
        }
        flush()
        trimPhrase()
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
        const val MAX_PHRASE = 120
        const val ENTER_MARK = '\u21B5'
    }
}
