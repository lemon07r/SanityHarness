package channelmultiplexer

import kotlinx.coroutines.*
import kotlinx.coroutines.channels.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class ChannelMultiplexerTest {

    @Test
    fun `single channel forwards values to output`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val input = Channel<Int>()
        
        mux.addChannel("test", input)
        
        launch {
            input.send(1)
            input.send(2)
            input.send(3)
            input.close()
        }
        
        val received = mutableListOf<Int>()
        repeat(3) {
            received.add(mux.output.receive())
        }
        
        assertEquals(listOf(1, 2, 3), received)
        mux.cancel()
    }

    @Test
    fun `multiple channels forward values to output`() = runTest {
        val mux = ChannelMultiplexer<String>(this)
        val ch1 = Channel<String>()
        val ch2 = Channel<String>()
        
        mux.addChannel("first", ch1)
        mux.addChannel("second", ch2)
        
        launch {
            ch1.send("a")
            ch2.send("b")
            ch1.send("c")
            ch2.send("d")
            ch1.close()
            ch2.close()
        }
        
        val received = mutableListOf<String>()
        repeat(4) {
            received.add(mux.output.receive())
        }
        
        // Order may vary, but all values should be received
        assertEquals(setOf("a", "b", "c", "d"), received.toSet())
        mux.cancel()
    }

    @Test
    fun `duplicate channel name throws exception`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch1 = Channel<Int>()
        val ch2 = Channel<Int>()
        
        mux.addChannel("same", ch1)
        
        assertThrows<IllegalArgumentException> {
            mux.addChannel("same", ch2)
        }
        
        mux.cancel()
    }

    @Test
    fun `cancel stops processing`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val input = Channel<Int>(Channel.UNLIMITED)
        
        mux.addChannel("test", input)
        input.send(1)
        
        assertEquals(1, mux.output.receive())
        
        mux.cancel()
        
        // Output should be closed after cancel
        assertTrue(mux.output.isClosedForReceive)
    }

    @Test
    fun `tagged multiplexer includes source name`() = runTest {
        val mux = TaggedChannelMultiplexer<Int>(this)
        val ch1 = Channel<Int>()
        val ch2 = Channel<Int>()
        
        mux.addChannel("sensor1", ch1)
        mux.addChannel("sensor2", ch2)
        
        launch {
            ch1.send(100)
            ch2.send(200)
            ch1.close()
            ch2.close()
        }
        
        val received = mutableListOf<TaggedValue<Int>>()
        repeat(2) {
            received.add(mux.output.receive())
        }
        
        // Check that each value has the correct source tag
        val sensor1Value = received.find { it.source == "sensor1" }
        val sensor2Value = received.find { it.source == "sensor2" }
        
        assertNotNull(sensor1Value)
        assertNotNull(sensor2Value)
        assertEquals(100, sensor1Value!!.value)
        assertEquals(200, sensor2Value!!.value)
        
        mux.cancel()
    }

    @Test
    fun `handles channel closure gracefully`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch1 = Channel<Int>()
        val ch2 = Channel<Int>()
        
        mux.addChannel("first", ch1)
        mux.addChannel("second", ch2)
        
        launch {
            ch1.send(1)
            ch1.close()  // Close one channel
            ch2.send(2)  // Other channel still works
            ch2.close()
        }
        
        val received = mutableListOf<Int>()
        repeat(2) {
            received.add(mux.output.receive())
        }
        
        assertEquals(setOf(1, 2), received.toSet())
        mux.cancel()
    }

    @Test
    fun `priority channel is received first when both have buffered values`() = runTest {
        val mux = ChannelMultiplexer<String>(this)
        val low = Channel<String>(Channel.UNLIMITED)
        val high = Channel<String>(Channel.UNLIMITED)

        mux.addChannel("low", low)
        mux.addPriorityChannel("high", high, priority = 10)

        low.send("low1")
        high.send("high1")

        val first = mux.output.receive()
        assertEquals("high1", first)

        mux.cancel()
    }

    @Test
    fun `removeChannel stops forwarding and updates count`() = runTest {
        val mux = ChannelMultiplexer<Int>(this)
        val ch1 = Channel<Int>(Channel.UNLIMITED)
        val ch2 = Channel<Int>(Channel.UNLIMITED)

        mux.addChannel("a", ch1)
        mux.addChannel("b", ch2)

        assertEquals(2, mux.getActiveChannelCount())
        assertTrue(mux.removeChannel("a"))
        assertEquals(1, mux.getActiveChannelCount())

        mux.cancel()
    }
}
