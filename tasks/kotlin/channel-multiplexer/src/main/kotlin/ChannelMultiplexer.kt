package channelmultiplexer

import kotlinx.coroutines.*
import kotlinx.coroutines.channels.*

data class TaggedValue<T>(val source: String, val value: T)

class ChannelMultiplexer<T>(private val scope: CoroutineScope) {
    private val out = Channel<T>(Channel.RENDEZVOUS)
    
    val output: ReceiveChannel<T> get() = out

    fun addChannel(name: String, channel: ReceiveChannel<T>) {
        scope.launch {
            for (v in channel) {
                out.send(v)
            }
        }
    }

    fun addPriorityChannel(name: String, channel: ReceiveChannel<T>, priority: Int) {
        addChannel(name, channel)
    }

    fun removeChannel(name: String): Boolean = false
    fun getActiveChannelCount(): Int = 0
    fun setBufferSize(size: Int) {}
    fun cancel() {}
}

class TaggedChannelMultiplexer<T>(private val scope: CoroutineScope) {
    private val out = Channel<TaggedValue<T>>(Channel.RENDEZVOUS)
    
    val output: ReceiveChannel<TaggedValue<T>> get() = out

    fun addChannel(name: String, channel: ReceiveChannel<T>) {
        scope.launch {
            for (v in channel) {
                out.send(TaggedValue(name, v))
            }
        }
    }

    fun addPriorityChannel(name: String, channel: ReceiveChannel<T>, priority: Int) {
        addChannel(name, channel)
    }

    fun removeChannel(name: String): Boolean = false
    fun getActiveChannelCount(): Int = 0
    fun setBufferSize(size: Int) {}
    fun cancel() {}
}
