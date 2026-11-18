package com.clipboardshare

/** Splits the typed phrase into the finished text and the word still being typed. */
object PhraseView {
    fun split(phrase: String): Pair<String, String> {
        val cut = phrase.indexOfLast { it.isWhitespace() || it == KeyboardTranslator.ENTER_MARK } + 1
        return phrase.substring(0, cut) to phrase.substring(cut)
    }
}
