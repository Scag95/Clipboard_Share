package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Test

class PhraseViewTest {
    @Test fun splitsTheLastWordFromTheRest() {
        assertEquals("hola " to "mundo", PhraseView.split("hola mundo"))
    }

    @Test fun aTrailingSpaceMeansThereIsNoCurrentWord() {
        assertEquals("hola " to "", PhraseView.split("hola "))
    }

    @Test fun anEmptyPhraseSplitsIntoNothing() {
        assertEquals("" to "", PhraseView.split(""))
    }

    @Test fun anEnterMarkSeparatesWordsLikeASpace() {
        assertEquals("hola" + KeyboardTranslator.ENTER_MARK to "ey", PhraseView.split("hola" + KeyboardTranslator.ENTER_MARK + "ey"))
    }

    @Test fun aSingleWordIsAllCurrent() {
        assertEquals("" to "hola", PhraseView.split("hola"))
    }
}
