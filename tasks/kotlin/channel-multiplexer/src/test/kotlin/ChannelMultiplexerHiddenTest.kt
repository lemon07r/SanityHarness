package channelmultiplexer

import kotlinx.coroutines.*
import kotlinx.coroutines.channels.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*
import kotlin.time.Duration.Companion.milliseconds

@OptIn(ExperimentalCoroutinesApi::class)
class ChannelMultiplexerHiddenTest {

    // ========== Priority Channel Tests ==========
    // Hidden requirement: addPriorityChannel(name, channel, priority) 
    // Higher priority channels should be processed first when multiple values are available

    @Test
    fun `priority channels are processed before normal channels`() = runTest {
        val mux = ChannelMultiplexer<String>(this)
        val lowPriority = Channel<String>(Channel.UNLIMITED)
        val highPriority = Channel<String>(Channel.UNLIMITED)
        
        // Add channels with different priorities
        mux.addChannel("low", lowPriority)
        mux.addPriorityChannel("high", highPriority, priority = 10)
        
        // Send to both channels before consuming
        lowPriority.send("low1")
        lowPriority.send("low2")
        highPriority.send("high1")
        highPriority.send("high2")
        
        // High priority should come first
        val first = mux.output.receive()
        val second = mux.output.receive()
        
        assertTrue(first.startsWith("high"), "Expected high priority first, got: $first")
        assertTrue(second.startsWith("high"), "Expected high priority second, got: $second")
        
        mux.cancel()
    }

    @Test
    fun `multiple priority levels are respected`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val low = Channel<Int>(Channel.UNLIMITED)
        val medium = Channel<Int>(Channel.UNLIMITED)
        val high = Channel<Int>(Channel.UNLIMITED)
        
        mux.addPriorityChannel("low", low, priority = 1)
        mux.addPriorityChannel("medium", medium, priority = 5)
        mux.addPriorityChannel("high", high, priority = 10)
        
        // Send one value to each
        low.send(1)
        medium.send(5)
        high.send(10)
        
        // Should receive in priority order: 10, 5, 1
        assertEquals(10, mux.output.receive())
        assertEquals(5, mux.output.receive())
        assertEquals(1, mux.output.receive())
        
        mux.cancel()
    }

    // ========== Remove Channel Tests ==========
    // Hidden requirement: removeChannel(name) - dynamically remove a channel

    @Test
    fun `removeChannel stops forwarding from that channel`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch1 = Channel<Int>(Channel.UNLIMITED)
        val ch2 = Channel<Int>(Channel.UNLIMITED)
        
        mux.addChannel("first", ch1)
        mux.addChannel("second", ch2)
        
        ch1.send(1)
        assertEquals(1, mux.output.receive())
        
        // Remove first channel
        mux.removeChannel("first")
        
        // Only second channel should work now
        ch2.send(2)
        assertEquals(2, mux.output.receive())
        
        // Sending to removed channel should not affect output
        // (implementation may close the channel or just stop processing)
        
        mux.cancel()
    }

    @Test
    fun `removeChannel returns true for existing channel`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch = Channel<Int>()
        
        mux.addChannel("test", ch)
        
        assertTrue(mux.removeChannel("test"))
        assertFalse(mux.removeChannel("nonexistent"))
        
        mux.cancel()
    }

    // ========== Active Channel Count Tests ==========
    // Hidden requirement: getActiveChannelCount() - return number of active channels

    @Test
    fun `getActiveChannelCount reflects added channels`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        
        assertEquals(0, mux.getActiveChannelCount())
        
        mux.addChannel("first", Channel())
        assertEquals(1, mux.getActiveChannelCount())
        
        mux.addChannel("second", Channel())
        assertEquals(2, mux.getActiveChannelCount())
        
        mux.addChannel("third", Channel())
        assertEquals(3, mux.getActiveChannelCount())
        
        mux.cancel()
    }

    @Test
    fun `getActiveChannelCount decreases when channels are removed`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch1 = Channel<Int>()
        val ch2 = Channel<Int>()
        
        mux.addChannel("first", ch1)
        mux.addChannel("second", ch2)
        assertEquals(2, mux.getActiveChannelCount())
        
        mux.removeChannel("first")
        assertEquals(1, mux.getActiveChannelCount())
        
        mux.removeChannel("second")
        assertEquals(0, mux.getActiveChannelCount())
        
        mux.cancel()
    }

    @Test
    fun `getActiveChannelCount decreases when channel closes`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch = Channel<Int>()
        
        mux.addChannel("test", ch)
        assertEquals(1, mux.getActiveChannelCount())
        
        ch.close()
        
        // Give time for the multiplexer to detect channel closure
        delay(50.milliseconds)
        
        assertEquals(0, mux.getActiveChannelCount())
        
        mux.cancel()
    }

    // ========== Buffer Size Tests ==========
    // Hidden requirement: setBufferSize(size) - configure output channel buffer

    @Test
    fun `setBufferSize configures output buffer`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        mux.setBufferSize(5)
        
        val input = Channel<Int>(Channel.UNLIMITED)
        mux.addChannel("test", input)
        
        // Should be able to buffer 5 items without blocking
        repeat(5) { i ->
            input.send(i)
        }
        
        // Verify all items can be received
        repeat(5) { i ->
            assertEquals(i, mux.output.receive())
        }
        
        mux.cancel()
    }

    // ========== Tagged Multiplexer Hidden Tests ==========

    @Test
    fun `tagged multiplexer supports priority channels`() = runTest {
        val mux = TaggedChannelMultiplexer<String>(this)
        val low = Channel<String>(Channel.UNLIMITED)
        val high = Channel<String>(Channel.UNLIMITED)
        
        mux.addChannel("low", low)
        mux.addPriorityChannel("high", high, priority = 10)
        
        low.send("low-value")
        high.send("high-value")
        
        val first = mux.output.receive()
        assertEquals("high", first.source)
        assertEquals("high-value", first.value)
        
        mux.cancel()
    }

    @Test
    fun `tagged multiplexer getActiveChannelCount works`() = runTest {
        val mux = TaggedChannelMultiplexer<Int>(this)
        
        assertEquals(0, mux.getActiveChannelCount())
        
        mux.addChannel("a", Channel())
        mux.addChannel("b", Channel())
        
        assertEquals(2, mux.getActiveChannelCount())
        
        mux.cancel()
    }

    // ========== Edge Cases ==========

    @Test
    fun `handles rapid add and remove`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        
        repeat(10) { i ->
            val ch = Channel<Int>(Channel.UNLIMITED)
            mux.addChannel("ch$i", ch)
            ch.send(i)
        }
        
        assertEquals(10, mux.getActiveChannelCount())
        
        repeat(5) { i ->
            mux.removeChannel("ch$i")
        }
        
        assertEquals(5, mux.getActiveChannelCount())
        
        mux.cancel()
    }

    @Test
    fun `priority zero is valid`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch = Channel<Int>(Channel.UNLIMITED)
        
        // Priority 0 should be valid (lowest priority)
        mux.addPriorityChannel("zero", ch, priority = 0)
        
        ch.send(42)
        assertEquals(42, mux.output.receive())
        
        mux.cancel()
    }

    @Test
    fun `negative priority throws exception`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch = Channel<Int>()
        
        assertThrows<IllegalArgumentException> {
            mux.addPriorityChannel("negative", ch, priority = -1)
        }
        
        mux.cancel()
    }
}
