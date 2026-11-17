package com.clipboardshare

/**
 * Decides when a clipboard-denial log line should bring up [ClipboardReadActivity], and
 * when the PC's clipboard must be held back because a phone copy is still being read.
 *
 * Pure Kotlin with an injectable monotonic [clock] (milliseconds), so the rules can be
 * unit-tested without Android. All methods are synchronized: the log watcher thread,
 * the main thread and the sync coroutines all call into it.
 */
class ClipboardTriggerPolicy(private val clock: () -> Long) {

    sealed class TriggerDecision {
        /** Bring the read window up; [readId] identifies this read for grace and settle. */
        data class LaunchRead(val readId: Long) : TriggerDecision()

        /** The system's echo of our own clipboard write. */
        object OwnChangeEcho : TriggerDecision()

        /** A real copy inside the quiet window: remembered, see [takeDeferredTrigger]. */
        object DeferredUntilQuietEnds : TriggerDecision()

        /** Too close to the previous trigger. */
        object Debounced : TriggerDecision()

        /** A read window is already up (or has not reported back yet). */
        object ReadInFlight : TriggerDecision()

        override fun toString(): String = javaClass.simpleName
    }

    companion object {
        /*
         * Timing constants. Their order matters:
         *
         *   TRIGGER_DEBOUNCE_MS < OWN_CHANGE_QUIET_MS
         *     The quiet window after our own write must outlast the debounce, and the
         *     delay between setPrimaryClip and the system logging the denial for it.
         *
         *   READ_WINDOW_LIMIT_MS < LOCAL_READ_PENDING_MS
         *     The PC clipboard stays held back for at least as long as the read window
         *     can live, so a remote apply can never land before the phone copy is read.
         *     (The hold-back is also released early once the read has been pushed.)
         *
         *   LOCAL_READ_PENDING_MS < READ_ACTIVITY_GRACE_MS
         *     The in-flight grace is only a fallback for a read window whose onDestroy
         *     never reported back; it must exceed the window's own hard limit so a live
         *     window is never doubled.
         */

        /** Ignore triggers this close to the previous one (one copy can log several lines). */
        const val TRIGGER_DEBOUNCE_MS = 400L

        /** After writing the clipboard ourselves, the next denial line is our own echo. */
        const val OWN_CHANGE_QUIET_MS = 1_500L

        /** Hard cap on how long ClipboardReadActivity may stay on screen. */
        const val READ_WINDOW_LIMIT_MS = 2_500L

        /** How long the PC's clipboard is held back while a phone copy is being read. */
        const val LOCAL_READ_PENDING_MS = 3_000L

        /** Fallback release of the in-flight guard if the read window never reports back. */
        const val READ_ACTIVITY_GRACE_MS = 5_000L

        /** A watcher that received nothing for this long probably lacks log access. */
        const val NO_LOG_LINES_HINT_AFTER_MS = 60_000L

        /**
         * Marker that ClipboardService (system_server, `system` log buffer, tag
         * `ClipboardService`, level E) prints when it refuses to hand a changed clip to
         * an app that is not in focus: "Denying clipboard access to <package>,
         * application is not in focus nor is it a system service for user <n>".
         * It is not an API: an Android update can reword it and silently disable the
         * background trigger.
         */
        const val CLIPBOARD_DENIAL_MARKER = "Denying clipboard access to "

        /** Printed by logcat itself before the first entry of a buffer. */
        private const val LOGCAT_HEADER_PREFIX = "--------- "

        private val EPOCH_TIMESTAMP = Regex("""^\s*(\d+)\.(\d{1,9})\s""")

        /** True when [line] is the denial marker addressed exactly to [packageName]. */
        fun isDenialFor(line: String, packageName: String): Boolean {
            val needle = CLIPBOARD_DENIAL_MARKER + packageName
            var index = line.indexOf(needle)
            while (index >= 0) {
                val after = index + needle.length
                if (after == line.length || !isPackageChar(line[after])) return true
                index = line.indexOf(needle, index + 1)
            }
            return false
        }

        /** Timestamp of a `logcat -v epoch` line in milliseconds, or null if it has none. */
        fun epochMillisOf(line: String): Long? {
            val match = EPOCH_TIMESTAMP.find(line) ?: return null
            val seconds = match.groupValues[1].toLongOrNull() ?: return null
            val millis = match.groupValues[2].padEnd(3, '0').take(3).toLong()
            return seconds * 1_000L + millis
        }

        private fun isPackageChar(c: Char): Boolean = c.isLetterOrDigit() || c == '.' || c == '_'
    }

    private var suppressTriggersUntil = Long.MIN_VALUE
    private var expectedOwnEchoes = 0
    private var deferredTrigger = false
    private var lastTriggerAt: Long? = null
    private var readInFlight = false
    private var readId = 0L
    private var localReadPendingUntil = Long.MIN_VALUE
    private var watcherStartedAt: Long? = null
    private var logLineArrived = false

    /**
     * Records that we are about to write the clipboard ourselves. The system logs a
     * denial for that write too (unless our app has focus: [expectEcho] false); it must
     * not bring up the read window. Returns the length of the quiet window, after which
     * [takeDeferredTrigger] must be checked.
     */
    @Synchronized
    fun onOwnClipWrite(expectEcho: Boolean = true): Long {
        val now = clock()
        if (now >= suppressTriggersUntil) expectedOwnEchoes = 0
        suppressTriggersUntil = now + OWN_CHANGE_QUIET_MS
        // With our app focused the system logs no denial for the write, so no echo is
        // coming; counting one would swallow a real copy made right after we leave.
        if (expectEcho) expectedOwnEchoes++
        return OWN_CHANGE_QUIET_MS
    }

    /** Decides what a fresh clipboard-denial line means. */
    @Synchronized
    fun onTrigger(): TriggerDecision {
        val now = clock()
        if (now < suppressTriggersUntil) {
            if (expectedOwnEchoes > 0) {
                expectedOwnEchoes--
                return TriggerDecision.OwnChangeEcho
            }
            // Our echo already arrived, so this is a real copy by the user: keep it.
            deferredTrigger = true
            return TriggerDecision.DeferredUntilQuietEnds
        }
        expectedOwnEchoes = 0
        val last = lastTriggerAt
        if (last != null && now - last < TRIGGER_DEBOUNCE_MS) return TriggerDecision.Debounced
        lastTriggerAt = now
        if (readInFlight) return TriggerDecision.ReadInFlight
        readInFlight = true
        readId++
        // The phone copy wins: hold the PC's clipboard back until it has been read and sent.
        localReadPendingUntil = now + LOCAL_READ_PENDING_MS
        return TriggerDecision.LaunchRead(readId)
    }

    /**
     * Called when the quiet window should have ended. True exactly once if a real copy
     * was deferred during it; the caller then treats it as a new trigger.
     */
    @Synchronized
    fun takeDeferredTrigger(): Boolean {
        if (clock() < suppressTriggersUntil || !deferredTrigger) return false
        deferredTrigger = false
        return true
    }

    /** The read window could not be started. */
    @Synchronized
    fun readLaunchFailed() {
        readInFlight = false
        localReadPendingUntil = Long.MIN_VALUE
    }

    /** The read window is gone. */
    @Synchronized
    fun readFinished() {
        readInFlight = false
    }

    /** Fallback release; ignored unless [id] is still the latest read. */
    @Synchronized
    fun readGraceExpired(id: Long) {
        if (id == readId) readInFlight = false
    }

    /** Id of the latest launched read, to settle it once its content has been handled. */
    @Synchronized
    fun latestReadId(): Long = readId

    /** The captured content of read [id] has been synced; release the hold-back. */
    @Synchronized
    fun localReadSettled(id: Long) {
        if (id == readId) localReadPendingUntil = Long.MIN_VALUE
    }

    /** Whether remote content must not be applied because a phone copy is being read. */
    @Synchronized
    fun isRemoteHeldBack(): Boolean = clock() < localReadPendingUntil

    /** A logcat process started; only the first start since [onWatcherStopped] counts. */
    @Synchronized
    fun onWatcherStarted() {
        if (watcherStartedAt == null) watcherStartedAt = clock()
    }

    @Synchronized
    fun onWatcherStopped() {
        watcherStartedAt = null
        logLineArrived = false
    }

    /**
     * Handles one logcat line. Returns true only for a clipboard denial addressed to
     * [packageName] that was logged at or after [watcherStartEpochMs]: `logcat -T 1`
     * replays the last old entry on every (re)start, and that must not trigger a read.
     * A line without a readable timestamp is accepted rather than silently dropped.
     */
    @Synchronized
    fun onLogLine(line: String, packageName: String, watcherStartEpochMs: Long): Boolean {
        if (line.startsWith(LOGCAT_HEADER_PREFIX) || line.isBlank()) return false
        logLineArrived = true
        if (!isDenialFor(line, packageName)) return false
        val loggedAt = epochMillisOf(line) ?: return true
        return loggedAt >= watcherStartEpochMs
    }

    /** True when the watcher has run for a while without receiving a single log line. */
    @Synchronized
    fun shouldHintNoLogLines(): Boolean {
        val startedAt = watcherStartedAt ?: return false
        return !logLineArrived && clock() - startedAt >= NO_LOG_LINES_HINT_AFTER_MS
    }
}
