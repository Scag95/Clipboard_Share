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

    @Test fun currentWordGrowsAsYouType() {
        translator.commit("hol")
        translator.commit("a")
        assertEquals("hola", translator.currentWord)
    }

    @Test fun spaceStartsANewWord() {
        translator.commit("hola ")
        assertEquals("", translator.currentWord)
        translator.commit("mun")
        assertEquals("mun", translator.currentWord)
    }

    @Test fun backspaceShortensTheWordAndStopsAtEmpty() {
        translator.commit("hola")
        translator.deleteBefore(1)
        assertEquals("hol", translator.currentWord)
        translator.deleteBefore(10)
        assertEquals("", translator.currentWord)
    }

    @Test fun enterAndNavigationKeysClearTheWord() {
        translator.commit("hola")
        translator.commit("\n")
        assertEquals("", translator.currentWord)
        translator.commit("hola")
        translator.pressKey("arrowleft")
        assertEquals("", translator.currentWord)
    }

    @Test fun composingRewriteKeepsTheLatestSpelling() {
        translator.setComposing("hol")
        translator.setComposing("hola")
        translator.setComposing("hole")
        assertEquals("hole", translator.currentWord)
    }

    @Test fun anEmojiIsDeletedAsOneCharacter() {
        translator.commit("a\uD83D\uDE00")
        translator.deleteBefore(1)
        assertEquals("a", translator.currentWord)
    }

    @Test fun listenerIsToldOnlyWhenTheWordChanges() {
        val seen = mutableListOf<String>()
        translator.onWordChanged = { seen += it }
        translator.commit("ab")
        translator.pressKey("arrowleft")
        translator.pressKey("arrowleft")
        assertEquals(listOf("ab", ""), seen)
    }

    @Test fun phraseKeepsEverythingTypedAndWordIsItsLastToken() {
        translator.commit("hola mundo")
        assertEquals("hola mundo", translator.currentPhrase)
        assertEquals("mundo", translator.currentWord)
    }

    @Test fun backspaceKeyEventShortensTheWordAndPhraseInsteadOfResettingThem() {
        translator.commit("hola")
        translator.pressKey("backspace")
        assertEquals("hol", translator.currentWord)
        assertEquals("hol", translator.currentPhrase)
        assertEquals("key:backspace", sent.last())
    }

    @Test fun enterIsMarkedInThePhraseAndEndsTheWord() {
        translator.commit("hola")
        translator.pressKey("enter")
        assertEquals("hola" + KeyboardTranslator.ENTER_MARK, translator.currentPhrase)
        assertEquals("", translator.currentWord)
    }

    @Test fun cursorKeysDropThePhraseBecauseThePcCursorMoved() {
        translator.commit("hola mundo")
        translator.pressKey("arrowleft")
        assertEquals("", translator.currentPhrase)
    }

    @Test fun phraseKeepsOnlyTheMostRecentCharacters() {
        translator.commit("x".repeat(KeyboardTranslator.MAX_PHRASE + 30) + "fin")
        assertEquals(KeyboardTranslator.MAX_PHRASE, translator.currentPhrase.length)
        assertTrue(translator.currentPhrase.endsWith("fin"))
    }

    @Test fun phraseListenerIsToldWhenItChanges() {
        val seen = mutableListOf<String>()
        translator.onPhraseChanged = { seen += it }
        translator.commit("a b")
        translator.deleteBefore(1)
        translator.pressKey("arrowleft")
        assertEquals(listOf("a b", "a ", ""), seen)
    }

    @Test fun resetTypingForgetsThePhraseAndTheWordWithoutTouchingThePc() {
        translator.commit("hola mundo")
        sent.clear()
        translator.resetTyping()
        assertEquals("", translator.currentPhrase)
        assertEquals("", translator.currentWord)
        assertTrue(sent.isEmpty())
        translator.commit("x")
        assertEquals("x", translator.currentPhrase)
    }

    @Test fun resetTypingTellsTheListenersAndIsSilentWhenAlreadyEmpty() {
        val phrases = mutableListOf<String>()
        translator.onPhraseChanged = { phrases += it }
        translator.commit("ab")
        translator.resetTyping()
        translator.resetTyping()
        assertEquals(listOf("ab", ""), phrases)
    }
}
