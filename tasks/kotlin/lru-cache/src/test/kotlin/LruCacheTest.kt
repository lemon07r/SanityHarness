package lrucache

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LruCacheTest {

    @Test
    fun `evicts least recently used on insert beyond capacity`() {
        val cache = LruCache<String, Int>(2)

        assertNull(cache.put("a", 1))
        assertNull(cache.put("b", 2))

        val evicted = cache.put("c", 3)
        assertEquals(1, evicted)

        assertNull(cache.get("a"))
        assertEquals(2, cache.get("b"))
        assertEquals(3, cache.get("c"))

        assertEquals(listOf("c", "b"), cache.keysMostRecentFirst())
    }

    @Test
    fun `get and overwrite update recency`() {
        val cache = LruCache<String, Int>(2)
        cache.put("a", 1)
        cache.put("b", 2)

        // Access a so b becomes least-recently-used.
        assertEquals(1, cache.get("a"))
        assertEquals(listOf("a", "b"), cache.keysMostRecentFirst())

        // Overwrite a should keep it most-recent.
        assertNull(cache.put("a", 10))
        assertEquals(listOf("a", "b"), cache.keysMostRecentFirst())

        // Inserting c should evict b.
        assertEquals(2, cache.put("c", 3))
        assertNull(cache.get("b"))
        assertEquals(listOf("c", "a"), cache.keysMostRecentFirst())
    }
}
