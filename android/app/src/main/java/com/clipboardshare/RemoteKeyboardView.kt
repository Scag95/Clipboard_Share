package com.clipboardshare

import android.content.Context
import android.text.InputType
import android.util.AttributeSet
import android.view.KeyEvent
import android.view.View
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager

/**
 * Invisible view that makes the soft keyboard behave like a keyboard attached
 * to the PC: every operation is forwarded as it happens and nothing is kept
 * or shown on the phone.
 */
class RemoteKeyboardView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
) : View(context, attrs) {
    var sink: KeyboardTranslator.Sink? = null

    private val translator = KeyboardTranslator(object : KeyboardTranslator.Sink {
        override fun text(chars: String): Boolean = sink?.text(chars) ?: false
        override fun key(name: String): Boolean = sink?.key(name) ?: false
    })

    /** Called with the word being typed (empty when none), on the main thread. */
    var onWordChanged: ((String) -> Unit)?
        get() = translator.onWordChanged
        set(value) { translator.onWordChanged = value }

    /** Called with the recent phrase (empty when none), on the main thread. */
    var onPhraseChanged: ((String) -> Unit)?
        get() = translator.onPhraseChanged
        set(value) { translator.onPhraseChanged = value }

    init {
        isFocusable = true
        isFocusableInTouchMode = true
    }

    override fun onCheckIsTextEditor(): Boolean = true

    fun show() {
        requestFocus()
        val imm = context.getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
        imm.restartInput(this)
        imm.showSoftInput(this, InputMethodManager.SHOW_IMPLICIT)
    }

    override fun onCreateInputConnection(outAttrs: EditorInfo): InputConnection {
        // No suggestions or autocorrect: the PC owns the text, so rewriting
        // earlier words would corrupt it.
        outAttrs.inputType = InputType.TYPE_CLASS_TEXT or
            InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD or
            InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        outAttrs.imeOptions = EditorInfo.IME_FLAG_NO_EXTRACT_UI or
            EditorInfo.IME_FLAG_NO_FULLSCREEN or EditorInfo.IME_ACTION_NONE
        return object : BaseInputConnection(this, false) {
            override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean =
                translator.commit(text)

            override fun setComposingText(text: CharSequence, newCursorPosition: Int): Boolean =
                translator.setComposing(text)

            override fun finishComposingText(): Boolean {
                translator.finishComposing()
                return true
            }

            override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean =
                if (beforeLength > 0) translator.deleteBefore(beforeLength) else true

            override fun deleteSurroundingTextInCodePoints(beforeLength: Int, afterLength: Int): Boolean =
                if (beforeLength > 0) translator.deleteBefore(beforeLength) else true

            override fun performEditorAction(actionCode: Int): Boolean =
                translator.pressKey("enter")

            override fun sendKeyEvent(event: KeyEvent): Boolean {
                if (event.action != KeyEvent.ACTION_DOWN) return true
                val name = keyName(event.keyCode)
                if (name != null) return translator.pressKey(name)
                val ch = event.unicodeChar
                return if (ch > 0) translator.commit(String(Character.toChars(ch))) else false
            }

            // The editor is empty by design; there is never text around the cursor.
            override fun getTextBeforeCursor(n: Int, flags: Int): CharSequence = ""
            override fun getTextAfterCursor(n: Int, flags: Int): CharSequence = ""
        }
    }

    private fun keyName(code: Int): String? = when (code) {
        KeyEvent.KEYCODE_DEL -> "backspace"
        KeyEvent.KEYCODE_FORWARD_DEL -> "delete"
        KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> "enter"
        KeyEvent.KEYCODE_TAB -> "tab"
        KeyEvent.KEYCODE_ESCAPE -> "esc"
        KeyEvent.KEYCODE_DPAD_LEFT -> "arrowleft"
        KeyEvent.KEYCODE_DPAD_RIGHT -> "arrowright"
        KeyEvent.KEYCODE_DPAD_UP -> "arrowup"
        KeyEvent.KEYCODE_DPAD_DOWN -> "arrowdown"
        else -> null
    }
}
