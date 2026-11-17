package com.clipboardshare

import com.clipboardshare.ClipboardTriggerPolicy.Companion.LOCAL_READ_PENDING_MS
import com.clipboardshare.ClipboardTriggerPolicy.Companion.NO_LOG_LINES_HINT_AFTER_MS
import com.clipboardshare.ClipboardTriggerPolicy.Companion.OWN_CHANGE_QUIET_MS
import com.clipboardshare.ClipboardTriggerPolicy.Companion.READ_ACTIVITY_GRACE_MS
import com.clipboardshare.ClipboardTriggerPolicy.Companion.READ_WINDOW_LIMIT_MS
import com.clipboardshare.ClipboardTriggerPolicy.Companion.TRIGGER_DEBOUNCE_MS
import com.clipboardshare.ClipboardTriggerPolicy.TriggerDecision
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ClipboardTriggerPolicyTest {

    private val pkg = "com.clipboardshare"
    private var now = 0L
    private lateinit var policy: ClipboardTriggerPolicy

    @Before
    fun setUp() {
        now = 10_000L
        policy = ClipboardTriggerPolicy(clock = { now })
    }

    private fun launch(): Long {
        val decision = policy.onTrigger()
        assertTrue("expected a launch, got $decision", decision is TriggerDecision.LaunchRead)
        return (decision as TriggerDecision.LaunchRead).readId
    }

    // --- Timing constants (A2) ---

    @Test
    fun timingConstantsKeepTheirDocumentedOrder() {
        assertTrue(TRIGGER_DEBOUNCE_MS < OWN_CHANGE_QUIET_MS)
        assertTrue(READ_WINDOW_LIMIT_MS < LOCAL_READ_PENDING_MS)
        assertTrue(LOCAL_READ_PENDING_MS < READ_ACTIVITY_GRACE_MS)
    }

    // --- Launch, debounce and in-flight guard ---

    @Test
    fun firstTriggerLaunchesAReadAndHoldsTheRemoteBack() {
        assertFalse(policy.isRemoteHeldBack())
        launch()
        assertTrue(policy.isRemoteHeldBack())
    }

    @Test
    fun triggerWithinTheDebounceIsDropped() {
        launch()
        policy.readFinished()
        now += TRIGGER_DEBOUNCE_MS - 1
        assertEquals(TriggerDecision.Debounced, policy.onTrigger())
    }

    @Test
    fun triggerAfterTheDebounceLaunchesAgainOnceTheReadFinished() {
        val first = launch()
        policy.readFinished()
        now += TRIGGER_DEBOUNCE_MS
        val second = launch()
        assertTrue(second != first)
    }

    @Test
    fun triggerWhileAReadIsInFlightIsRejected() {
        launch()
        now += TRIGGER_DEBOUNCE_MS
        assertEquals(TriggerDecision.ReadInFlight, policy.onTrigger())
    }

    @Test
    fun graceExpiryReleasesAReadThatNeverReportedBack() {
        val readId = launch()
        now += READ_ACTIVITY_GRACE_MS
        policy.readGraceExpired(readId)
        launch()
    }

    @Test
    fun staleGraceFromAnEarlierReadDoesNotReleaseALaterRead() {
        val first = launch()
        policy.readFinished()
        now += TRIGGER_DEBOUNCE_MS
        launch()
        policy.readGraceExpired(first)
        now += TRIGGER_DEBOUNCE_MS
        assertEquals(TriggerDecision.ReadInFlight, policy.onTrigger())
    }

    @Test
    fun launchFailureReleasesTheGuardAndTheHoldBack() {
        launch()
        policy.readLaunchFailed()
        assertFalse(policy.isRemoteHeldBack())
        now += TRIGGER_DEBOUNCE_MS
        launch()
    }

    // --- Remote hold-back while a local read is pending ---

    @Test
    fun remoteHoldBackEndsWhenThePendingWindowExpires() {
        launch()
        now += LOCAL_READ_PENDING_MS - 1
        assertTrue(policy.isRemoteHeldBack())
        now += 1
        assertFalse(policy.isRemoteHeldBack())
    }

    @Test
    fun settlingTheLatestReadReleasesTheRemote() {
        val readId = launch()
        policy.localReadSettled(readId)
        assertFalse(policy.isRemoteHeldBack())
    }

    @Test
    fun settlingAnEarlierReadKeepsANewerReadHoldingTheRemoteBack() {
        val first = launch()
        policy.readFinished()
        now += TRIGGER_DEBOUNCE_MS
        val second = launch()
        policy.localReadSettled(first)
        assertTrue(policy.isRemoteHeldBack())
        assertEquals(second, policy.latestReadId())
    }

    // --- Own-change suppression and the deferred user copy (A5) ---

    @Test
    fun echoOfOwnWriteIsSuppressedAndNothingIsDeferred() {
        val quietMs = policy.onOwnClipWrite()
        assertEquals(OWN_CHANGE_QUIET_MS, quietMs)
        assertEquals(TriggerDecision.OwnChangeEcho, policy.onTrigger())
        assertFalse(policy.isRemoteHeldBack())
        now += OWN_CHANGE_QUIET_MS
        assertFalse(policy.takeDeferredTrigger())
    }

    @Test
    fun userCopyDuringTheQuietWindowFiresOnceWhenTheWindowEnds() {
        policy.onOwnClipWrite()
        assertEquals(TriggerDecision.OwnChangeEcho, policy.onTrigger())
        now += 100
        assertEquals(TriggerDecision.DeferredUntilQuietEnds, policy.onTrigger())
        assertFalse(policy.takeDeferredTrigger())
        now += OWN_CHANGE_QUIET_MS
        assertTrue(policy.takeDeferredTrigger())
        assertFalse(policy.takeDeferredTrigger())
        launch()
    }

    @Test
    fun deferredTriggerWaitsForAQuietWindowExtendedByAnotherOwnWrite() {
        policy.onOwnClipWrite()
        policy.onTrigger()
        policy.onTrigger()
        now += OWN_CHANGE_QUIET_MS - 100
        policy.onOwnClipWrite()
        now += 100
        assertFalse(policy.takeDeferredTrigger())
        now += OWN_CHANGE_QUIET_MS
        assertTrue(policy.takeDeferredTrigger())
    }

    @Test
    fun triggerAfterTheQuietWindowLaunchesEvenIfTheEchoNeverCame() {
        policy.onOwnClipWrite()
        now += OWN_CHANGE_QUIET_MS
        launch()
    }

    // --- Log lines (A8, A9) ---

    private fun denial(epochSeconds: String, target: String = pkg) =
        "$epochSeconds  1000  1234 E ClipboardService: Denying clipboard access to $target, " +
            "application is not in focus nor is it a system service for user 0"

    @Test
    fun denialLoggedBeforeTheWatcherStartedIsIgnored() {
        assertFalse(policy.onLogLine(denial("1700000000.000"), pkg, watcherStartEpochMs = 1_700_000_000_500L))
    }

    @Test
    fun denialLoggedAfterTheWatcherStartedIsAccepted() {
        assertTrue(policy.onLogLine(denial("1700000000.500"), pkg, watcherStartEpochMs = 1_700_000_000_500L))
        assertTrue(policy.onLogLine(denial("   1700000001.000"), pkg, watcherStartEpochMs = 1_700_000_000_500L))
    }

    @Test
    fun denialForAnotherPackageIsIgnored() {
        assertFalse(policy.onLogLine(denial("1700000001.000", "com.other"), pkg, 1_700_000_000_000L))
        assertFalse(policy.onLogLine(denial("1700000001.000", "com.clipboardshare.debug"), pkg, 1_700_000_000_000L))
    }

    @Test
    fun denialWithoutAReadableTimestampIsAccepted() {
        assertTrue(
            policy.onLogLine(
                "E/ClipboardService( 1234): Denying clipboard access to $pkg, application is not in focus",
                pkg,
                1_700_000_000_000L,
            ),
        )
    }

    @Test
    fun noLogHintOnlyAfterTheWatcherRanSilentlyForTheThreshold() {
        assertFalse(policy.shouldHintNoLogLines())
        policy.onWatcherStarted()
        now += NO_LOG_LINES_HINT_AFTER_MS - 1
        assertFalse(policy.shouldHintNoLogLines())
        now += 1
        assertTrue(policy.shouldHintNoLogLines())
    }

    @Test
    fun anyLogLineEvenAStaleOneClearsTheNoLogHint() {
        policy.onWatcherStarted()
        policy.onLogLine(denial("1600000000.000"), pkg, 1_700_000_000_000L)
        now += NO_LOG_LINES_HINT_AFTER_MS
        assertFalse(policy.shouldHintNoLogLines())
    }

    @Test
    fun logcatBufferHeaderDoesNotCountAsALogLine() {
        policy.onWatcherStarted()
        assertFalse(policy.onLogLine("--------- beginning of system", pkg, 1_700_000_000_000L))
        now += NO_LOG_LINES_HINT_AFTER_MS
        assertTrue(policy.shouldHintNoLogLines())
    }

    @Test
    fun watcherRestartKeepsTheOriginalStartForTheHint() {
        policy.onWatcherStarted()
        now += NO_LOG_LINES_HINT_AFTER_MS - 1
        policy.onWatcherStarted()
        now += 1
        assertTrue(policy.shouldHintNoLogLines())
    }

    @Test
    fun stoppingTheWatcherClearsTheHint() {
        policy.onWatcherStarted()
        now += NO_LOG_LINES_HINT_AFTER_MS
        policy.onWatcherStopped()
        assertFalse(policy.shouldHintNoLogLines())
    }
}
