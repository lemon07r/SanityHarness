package flowprocessor

import kotlinx.coroutines.flow.Flow

/**
 * A composable flow processor that can transform, filter, and aggregate data streams.
 */
interface FlowProcessor<T, R> {
    /**
     * Processes the input flow and returns a transformed flow.
     */
    fun process(input: Flow<T>): Flow<R>
}

/**
 * Creates a mapping processor that transforms each element.
 */
fun <T, R> map(transform: suspend (T) -> R): FlowProcessor<T, R> {
    TODO("Implement map processor")
}

/**
 * Creates a filtering processor that only emits elements matching the predicate.
 */
fun <T> filter(predicate: suspend (T) -> Boolean): FlowProcessor<T, T> {
    TODO("Implement filter processor")
}

/**
 * Creates a processor that flattens nested flows.
 */
fun <T, R> flatMap(transform: suspend (T) -> Flow<R>): FlowProcessor<T, R> {
    TODO("Implement flatMap processor")
}

/**
 * Composes two processors into a single processor.
 */
infix fun <T, R, S> FlowProcessor<T, R>.then(other: FlowProcessor<R, S>): FlowProcessor<T, S> {
    TODO("Implement processor composition")
}

/**
 * Creates a processor that batches elements into lists of the given size.
 *
 * Contract:
 * - Emits lists of exactly [size] elements while enough input exists.
 * - Emits one final partial batch if input completes with leftover elements.
 */
fun <T> batch(size: Int): FlowProcessor<T, List<T>> {
    TODO("Implement batch processor")
}

/**
 * Creates a processor that handles errors with a recovery function.
 *
 * Contract:
 * - When processing an element throws, emit `handler(error)` for that failure.
 * - Continue processing subsequent elements after recovery.
 */
fun <T> recover(handler: suspend (Throwable) -> T): FlowProcessor<T, T> {
    TODO("Implement recover processor")
}
