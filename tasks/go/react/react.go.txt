// Package react implements reactive cells.
package react

// Reactor is the container for reactive cells.
type Reactor interface {
	// CreateInput creates an input cell with the given initial value.
	CreateInput(int) InputCell
	// CreateCompute1 creates a compute cell that depends on one cell.
	CreateCompute1(Cell, func(int) int) ComputeCell
	// CreateCompute2 creates a compute cell that depends on two cells.
	CreateCompute2(Cell, Cell, func(int, int) int) ComputeCell
}

// Cell is the base interface for all cells.
type Cell interface {
	Value() int
}

// InputCell is a cell whose value can be set.
type InputCell interface {
	Cell
	SetValue(int)
}

// ComputeCell is a cell whose value is computed from other cells.
type ComputeCell interface {
	Cell
	// AddCallback adds a callback that fires when the value changes.
	// Returns a function to remove the callback.
	AddCallback(func(int)) func()
}

// New creates a new Reactor.
func New() Reactor {
	panic("Please implement New")
}
