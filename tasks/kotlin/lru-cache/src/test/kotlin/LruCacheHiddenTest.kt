package lrucache

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LruCacheHiddenTest {

    @Test
    fun `capacity must be positive`() {
        assertThrows(IllegalArgumentException::class.java) {
            LruCache<String, Int>(0)
        }
        assertThrows(IllegalArgumentException::class.java) {
            LruCache<String, Int>(-1)
        }
    }

    @Test
    fun `remove returns previous value and updates recency`() {
        val cache = LruCache<String, Int>(2)
        cache.put("a", 1)
        cache.put("b", 2)

        assertEquals(1, cache.remove("a"))
        assertNull(cache.get("a"))
        assertEquals(listOf("b"), cache.keysMostRecentFirst())

        assertNull(cache.remove("missing"))
    }

    @Test
    fun `repeated gets reorder keys`() {
        val cache = LruCache<String, Int>(3)
        cache.put("a", 1)
        cache.put("b", 2)
        cache.put("c", 3)

        assertEquals(listOf("c", "b", "a"), cache.keysMostRecentFirst())

        cache.get("a")
        assertEquals(listOf("a", "c", "b"), cache.keysMostRecentFirst())

        cache.get("b")
        assertEquals(listOf("b", "a", "c"), cache.keysMostRecentFirst())
    }
}
