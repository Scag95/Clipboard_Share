package com.clipboardshare

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** S3 (#12): the app pairs and syncs only with a PC that speaks its protocol. */
class ProtocolCompatTest {
    @Test
    fun appSpeaksProtocolOne() {
        assertEquals(1, ProtocolCompat.APP_PROTOCOL)
    }

    @Test
    fun sameProtocolIsCompatible() {
        assertEquals(ProtocolCompat.Result.COMPATIBLE, ProtocolCompat.check(1))
        assertEquals(ProtocolCompat.Result.COMPATIBLE, ProtocolCompat.check(3, appProtocol = 3))
    }

    @Test
    fun missingProtocolIsAnOlderServer() {
        assertEquals(ProtocolCompat.Result.SERVER_OLDER, ProtocolCompat.check(null))
    }

    @Test
    fun lowerProtocolIsAnOlderServer() {
        assertEquals(ProtocolCompat.Result.SERVER_OLDER, ProtocolCompat.check(0))
        assertEquals(ProtocolCompat.Result.SERVER_OLDER, ProtocolCompat.check(1, appProtocol = 2))
    }

    @Test
    fun higherProtocolIsANewerServer() {
        assertEquals(ProtocolCompat.Result.SERVER_NEWER, ProtocolCompat.check(2))
        assertEquals(ProtocolCompat.Result.SERVER_NEWER, ProtocolCompat.check(5, appProtocol = 4))
    }

    @Test
    fun compatibleHasNoMessage() {
        assertNull(ProtocolCompat.message(1))
    }

    @Test
    fun missingProtocolMessageSaysUnknownAndToUpdateThePc() {
        assertEquals(
            "La PC usa otra versión de Nudge (protocolo desconocido; esta app usa 1). " +
                "Actualizá la app o el servidor para que coincidan. Actualizá Nudge en la PC.",
            ProtocolCompat.message(null),
        )
    }

    @Test
    fun olderServerMessageNamesBothProtocolsAndToUpdateThePc() {
        assertEquals(
            "La PC usa otra versión de Nudge (protocolo 0; esta app usa 1). " +
                "Actualizá la app o el servidor para que coincidan. Actualizá Nudge en la PC.",
            ProtocolCompat.message(0),
        )
    }

    @Test
    fun newerServerMessageNamesBothProtocolsAndToUpdateTheApp() {
        assertEquals(
            "La PC usa otra versión de Nudge (protocolo 2; esta app usa 1). " +
                "Actualizá la app o el servidor para que coincidan. Actualizá la app.",
            ProtocolCompat.message(2),
        )
    }

    @Test
    fun protocolFieldIsReadOnlyFromIntegralNumbers() {
        assertEquals(1, ProtocolCompat.protocolOf(1))
        assertEquals(2, ProtocolCompat.protocolOf(2L))
        assertEquals(1, ProtocolCompat.protocolOf(1.0))
        assertNull(ProtocolCompat.protocolOf(null))
        assertNull(ProtocolCompat.protocolOf("1"))
        assertNull(ProtocolCompat.protocolOf(1.5))
        assertNull(ProtocolCompat.protocolOf(true))
    }

    @Test
    fun unreachableProbeDoesNotBlock() {
        assertNull(ProtocolCompat.blockingMessage(ProtocolCompat.HealthProbe.Unreachable))
    }

    @Test
    fun compatibleProbeDoesNotBlock() {
        assertNull(ProtocolCompat.blockingMessage(ProtocolCompat.HealthProbe.Answered(1)))
    }

    @Test
    fun probeWithoutProtocolBlocksWithTheUpdatePcMessage() {
        assertEquals(
            ProtocolCompat.message(null),
            ProtocolCompat.blockingMessage(ProtocolCompat.HealthProbe.Answered(null)),
        )
    }

    @Test
    fun probeWithAnotherProtocolBlocksWithItsMessage() {
        assertEquals(
            ProtocolCompat.message(2),
            ProtocolCompat.blockingMessage(ProtocolCompat.HealthProbe.Answered(2)),
        )
        assertEquals(
            ProtocolCompat.message(0),
            ProtocolCompat.blockingMessage(ProtocolCompat.HealthProbe.Answered(0)),
        )
    }

    @Test
    fun syncProbesOnStartAndAfterAFailedPollOnly() {
        assertTrue(ProtocolCompat.probeBeforePoll(firstPoll = true, lastPollReachedServer = true))
        assertTrue(ProtocolCompat.probeBeforePoll(firstPoll = true, lastPollReachedServer = false))
        assertTrue(ProtocolCompat.probeBeforePoll(firstPoll = false, lastPollReachedServer = false))
        assertFalse(ProtocolCompat.probeBeforePoll(firstPoll = false, lastPollReachedServer = true))
    }
}
