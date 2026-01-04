package flowprocessor

import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class FlowProcessorTest {
    
    @Test
    fun `map transforms elements`() = runBlocking {
        val processor = map<Int, Int> { it * 2 }
        val result = processor.process(flowOf(1, 2, 3)).toList()
        assertEquals(listOf(2, 4, 6), result)
    }
    
    @Test
    fun `filter removes non-matching elements`() = runBlocking {
        val processor = filter<Int> { it % 2 == 0 }
        val result = processor.process(flowOf(1, 2, 3, 4, 5)).toList()
        assertEquals(listOf(2, 4), result)
    }
    
    @Test
    fun `flatMap expands elements`() = runBlocking {
        val processor = flatMap<Int, Int> { flowOf(it, it * 10) }
        val result = processor.process(flowOf(1, 2)).toList()
        assertEquals(listOf(1, 10, 2, 20), result)
    }
    
    @Test
    fun `processors can be composed`() = runBlocking {
        val double = map<Int, Int> { it * 2 }
        val addOne = map<Int, Int> { it + 1 }
        val composed = double then addOne
        
        val result = composed.process(flowOf(1, 2, 3)).toList()
        assertEquals(listOf(3, 5, 7), result)
    }
    
    @Test
    fun `batch groups elements`() = runBlocking {
        val processor = batch<Int>(3)
        val result = processor.process(flowOf(1, 2, 3, 4, 5)).toList()
        assertEquals(listOf(listOf(1, 2, 3), listOf(4, 5)), result)
    }
    
    @Test
    fun `recover handles errors`() = runBlocking {
        val processor = map<Int, Int> { 
            if (it == 2) throw RuntimeException("Error on 2")
            it 
        } then recover { -1 }
        
        val result = processor.process(flowOf(1, 2, 3)).toList()
        assertEquals(listOf(1, -1, 3), result)
    }
    
    @Test
    fun `empty flow produces empty result`() = runBlocking {
        val processor = map<Int, Int> { it * 2 }
        val result = processor.process(flowOf<Int>()).toList()
        assertTrue(result.isEmpty())
    }
    
    @Test
    fun `multiple compositions work correctly`() = runBlocking {
        val pipeline = filter<Int> { it > 0 } then 
                       map { it * 2 } then 
                       filter { it < 10 }
        
        val result = pipeline.process(flowOf(-1, 1, 2, 5, 10)).toList()
        assertEquals(listOf(2, 4), result)
    }
}
