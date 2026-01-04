package flowprocessor

import kotlinx.coroutines.flow.asFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class FlowProcessorHiddenTest {

    @Test
    fun `recover handles multiple errors and continues`() = runBlocking {
        val pipeline = map<Int, Int> {
            if (it == 2) throw RuntimeException("boom")
            it
        } then recover { -1 }

        val result = pipeline.process(listOf(1, 2, 2, 3).asFlow()).toList()
        assertEquals(listOf(1, -1, -1, 3), result)
    }

    @Test
    fun `batch emits exact sized batches when divisible`() = runBlocking {
        val processor = batch<Int>(3)
        val result = processor.process((1..6).asFlow()).toList()
        assertEquals(listOf(listOf(1, 2, 3), listOf(4, 5, 6)), result)
    }

    @Test
    fun `composition preserves element order`() = runBlocking {
        val pipeline = filter<Int> { it >= 0 } then map { it * 2 }
        val result = pipeline.process(flowOf(-1, 0, 1, 2)).toList()
        assertEquals(listOf(0, 2, 4), result)
    }
}
