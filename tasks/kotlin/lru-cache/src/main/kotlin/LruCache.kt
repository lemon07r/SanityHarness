package lrucache

/**
 * A fixed-capacity Least Recently Used (LRU) cache.
 *
 * - [get] marks the entry as most recently used.
 * - [put] inserts/updates an entry and may evict the least recently used entry.
 */
class LruCache<K, V>(capacity: Int) {
    init {
        // TODO: validate capacity
    }

    /**
     * Returns the value for [key], or null if missing.
     * Marks the entry as most recently used when present.
     */
    fun get(key: K): V? {
        TODO("Implement get")
    }

    /**
     * Inserts or updates [key] with [value].
     * Returns the *evicted* value if an eviction occurred, otherwise null.
     */
    fun put(key: K, value: V): V? {
        TODO("Implement put")
    }

    /**
     * Removes [key] from the cache and returns its previous value, or null.
     */
    fun remove(key: K): V? {
        TODO("Implement remove")
    }

    /**
     * Returns cache keys ordered from most-recently-used to least-recently-used.
     */
    fun keysMostRecentFirst(): List<K> {
        TODO("Implement keysMostRecentFirst")
    }
}
