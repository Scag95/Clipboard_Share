package com.clipboardshare

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.RectF
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import kotlin.math.abs

/**
 * Touchpad gesture surface (S2): one finger moves the PC pointer, a quick
 * single tap is a left click, a long press or a two-finger tap is a right
 * click, and dragging two fingers scrolls. Events are relative deltas; the
 * view never tracks an absolute PC cursor position.
 */
class TouchpadView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
) : View(context, attrs) {

    interface Listener {
        fun onMove(dx: Int, dy: Int)
        fun onLeftTap()
        fun onRightTap()
        fun onScroll(dxNotches: Int, dyNotches: Int)
    }

    var listener: Listener? = null

    private val slop = ViewConfiguration.get(context).scaledTouchSlop
    private val notchPx = 36f * resources.displayMetrics.density
    private val longPressMs = 600L
    private val tapMs = 300L
    private val twoFingerTapMs = 500L

    private val stripPx = 52.8f * resources.displayMetrics.density
    private var edgeScroll = false
    private val stripPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = 0xFF45454D.toInt() }
    private val panelPaint = Paint().apply { color = 0x1AFFFFFF }
    private val dividerPaint = Paint().apply { color = 0xFF45454D.toInt() }
    private val stripRect = RectF()

    private var primaryId = -1
    private var activeCount = 0
    private var lastX = 0f
    private var lastY = 0f
    private var downTime = 0L
    private var totalMove = 0f
    private var twoFingerMove = 0f
    private var twoFinger = false
    private var gestureWasTwo = false
    private var consumed = false
    private var scrollAccumX = 0f
    private var scrollAccumY = 0f

    private val longPressRunnable = Runnable {
        if (!twoFinger && activeCount == 1 && !consumed) {
            listener?.onRightTap()
            consumed = true
            performClick()
        }
    }

    init {
        if (background == null) setBackgroundResource(R.drawable.trackpad_surface)
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                primaryId = event.getPointerId(0)
                lastX = event.x
                lastY = event.y
                downTime = System.currentTimeMillis()
                activeCount = 1
                totalMove = 0f
                twoFingerMove = 0f
                twoFinger = false
                gestureWasTwo = false
                consumed = false
                scrollAccumX = 0f
                scrollAccumY = 0f
                edgeScroll = EdgeScroll.inStrip(event.x, width, stripPx)
                if (!edgeScroll) postDelayed(longPressRunnable, longPressMs)
            }

            MotionEvent.ACTION_POINTER_DOWN -> {
                activeCount = event.pointerCount
                if (activeCount >= 2) {
                    edgeScroll = false
                    twoFinger = true
                    gestureWasTwo = true
                    removeCallbacks(longPressRunnable)
                    val c = centroid(event)
                    lastX = c[0]
                    lastY = c[1]
                }
            }

            MotionEvent.ACTION_MOVE -> {
                if (twoFinger) {
                    if (event.pointerCount < 2) return true
                    val c = centroid(event)
                    val dx = c[0] - lastX
                    val dy = c[1] - lastY
                    lastX = c[0]
                    lastY = c[1]
                    twoFingerMove += abs(dx) + abs(dy)
                    scrollAccumX += dx
                    scrollAccumY += dy
                    // Phone drag down = wheel down on the PC (content scrolls
                    // toward the user, matching natural scrolling).
                    val nx = (scrollAccumX / notchPx).toInt()
                    val ny = (scrollAccumY / notchPx).toInt()
                    if (nx != 0 || ny != 0) {
                        listener?.onScroll(nx, ny)
                        scrollAccumX -= nx * notchPx
                        scrollAccumY -= ny * notchPx
                    }
                } else if (activeCount == 1) {
                    val idx = event.findPointerIndex(primaryId)
                    if (idx < 0) return true
                    val x = event.getX(idx)
                    val y = event.getY(idx)
                    val dx = x - lastX
                    val dy = y - lastY
                    lastX = x
                    lastY = y
                    if (edgeScroll) {
                        // Same direction as the two-finger scroll.
                        scrollAccumY += dy
                        val ny = (scrollAccumY / notchPx).toInt()
                        if (ny != 0) {
                            listener?.onScroll(0, ny)
                            scrollAccumY -= ny * notchPx
                        }
                        return true
                    }
                    if (dx != 0f || dy != 0f) {
                        listener?.onMove(dx.toInt(), dy.toInt())
                        // One finger still moving after a two-finger gesture is
                        // not a stationary two-finger tap.
                        if (gestureWasTwo) twoFingerMove += abs(dx) + abs(dy)
                    }
                    totalMove += abs(dx) + abs(dy)
                    if (totalMove > slop) removeCallbacks(longPressRunnable)
                }
            }

            MotionEvent.ACTION_POINTER_UP -> {
                activeCount = event.pointerCount - 1
                if (activeCount < 2) {
                    twoFinger = false
                    val remIdx = if (event.actionIndex == 0) 1 else 0
                    if (remIdx < event.pointerCount) {
                        primaryId = event.getPointerId(remIdx)
                        lastX = event.getX(remIdx)
                        lastY = event.getY(remIdx)
                    }
                    scrollAccumX = 0f
                    scrollAccumY = 0f
                }
            }

            MotionEvent.ACTION_UP -> {
                removeCallbacks(longPressRunnable)
                val dur = System.currentTimeMillis() - downTime
                if (!consumed && !edgeScroll) {
                    when {
                        gestureWasTwo && dur < twoFingerTapMs && twoFingerMove <= slop -> {
                            listener?.onRightTap()
                            performClick()
                        }
                        !gestureWasTwo && dur < tapMs && totalMove <= slop -> {
                            listener?.onLeftTap()
                            performClick()
                        }
                    }
                }
                resetState()
            }

            MotionEvent.ACTION_CANCEL -> resetState()
        }
        return true
    }

    override fun performClick(): Boolean {
        super.performClick()
        return true
    }

    private fun centroid(event: MotionEvent): FloatArray {
        var sx = 0f
        var sy = 0f
        for (i in 0 until event.pointerCount) {
            sx += event.getX(i)
            sy += event.getY(i)
        }
        val n = event.pointerCount.toFloat()
        return floatArrayOf(sx / n, sy / n)
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        // The scroll strip fills the full height: a faint panel, a divider and a rail.
        val density = resources.displayMetrics.density
        val left = width - stripPx
        canvas.drawRect(left, 0f, width.toFloat(), height.toFloat(), panelPaint)
        canvas.drawRect(left, 0f, left + density, height.toFloat(), dividerPaint)
        val barW = 4f * density
        val cx = width - stripPx / 2f
        stripRect.set(cx - barW / 2f, height * 0.1f, cx + barW / 2f, height * 0.9f)
        canvas.drawRoundRect(stripRect, barW / 2f, barW / 2f, stripPaint)
    }

    private fun resetState() {
        edgeScroll = false
        activeCount = 0
        twoFinger = false
        totalMove = 0f
        twoFingerMove = 0f
        scrollAccumX = 0f
        scrollAccumY = 0f
    }
}
