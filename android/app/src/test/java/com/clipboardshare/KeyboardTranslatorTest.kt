package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class KeyboardTranslatorTest {
    private val sent = mutableListOf<String>()
    private var accept = true
    private val translator = KeyboardTranslator(object : KeyboardTranslator.Sink {
        override fun text(chars: String): Boolean { sent += "text:$chars"; return accept }
        override fun key(name: String): Boolean { sent += "key:$name"; return accept }
    })

    @Test fun commitSendsTextAsTyped() {
        assertTrue(translator.commit("hola"))
        assertEquals(listOf("text:hola"), sent)
    }

    @Test fun newlineBecomesEnterAndSplitsText() {
        translator.commit("a\nb")
        assertEquals(listOf("text:a", "key:enter", "text:b"), sent)
    }

    @Test fun otherControlCharactersAreDropped() {
        translator.commit("a\u0007b\t")
        assertEquals(listOf("text:ab"), sent)
    }

    @Test fun deleteSendsOneBackspacePerCount() {
        translator.deleteBefore(3)
        assertEquals(List(3) { "key:backspace" }, sent)
    }

    @Test fun deleteBeyondRateBudgetIsRejectedWithoutSending() {
        assertFalse(translator.deleteBefore(KeyboardTranslator.MAX_BACKSPACES + 1))
        assertTrue(sent.isEmpty())
    }

    @Test fun longTextIsChunkedBelowTheServerLimit() {
        translator.commit("x".repeat(2500))
        assertEquals(listOf(1000, 1000, 500), sent.map { it.removePrefix("text:").length })
    }

    @Test fun composingRewritesOnlyTheChangedTail() {
        translator.setComposing("hol")
        translator.setComposing("hola")
        translator.setComposing("hole")
        assertEquals(
            listOf("text:hol", "text:a", "key:backspace", "text:e"),
            sent,
        )
    }

    @Test fun commitAfterComposingDoesNotRetypeThePrefix() {
        translator.setComposing("hol")
        translator.commit("hola")
        assertEquals(listOf("text:hol", "text:a"), sent)
        sent.clear()
        translator.commit("x")
        assertEquals(listOf("text:x"), sent)
    }

    @Test fun finishComposingKeepsTextAndForgetsIt() {
        translator.setComposing("ab")
        translator.finishComposing()
        translator.setComposing("c")
        assertEquals(listOf("text:ab", "text:c"), sent)
    }

    @Test fun emojiCountsAsOneBackspaceWhenRewritingComposition() {
        translator.setComposing("a\uD83D\uDE00")
        translator.setComposing("a")
        assertEquals(listOf("text:a\uD83D\uDE00", "key:backspace"), sent)
    }

    @Test fun failedSendIsReportedToTheCaller() {
        accept = false
        assertFalse(translator.commit("a"))
        assertFalse(translator.deleteBefore(1))
    }

    @Test fun namedKeysPassThrough() {
        assertTrue(translator.pressKey("arrowleft"))
        assertEquals(listOf("key:arrowleft"), sent)
    }
}
