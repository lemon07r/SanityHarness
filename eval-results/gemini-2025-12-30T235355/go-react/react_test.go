package react

import "testing"

func TestInputCell(t *testing.T) {
	r := New()
	input := r.CreateInput(10)
	if input.Value() != 10 {
		t.Fatalf("input.Value() = %d, want 10", input.Value())
	}
}

func TestInputCellSetValue(t *testing.T) {
	r := New()
	input := r.CreateInput(10)
	input.SetValue(20)
	if input.Value() != 20 {
		t.Fatalf("input.Value() = %d, want 20", input.Value())
	}
}

func TestComputeCell1(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	compute := r.CreateCompute1(input, func(v int) int { return v + 1 })
	if compute.Value() != 2 {
		t.Fatalf("compute.Value() = %d, want 2", compute.Value())
	}
}

func TestComputeCell1Updates(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	compute := r.CreateCompute1(input, func(v int) int { return v + 1 })
	input.SetValue(3)
	if compute.Value() != 4 {
		t.Fatalf("compute.Value() = %d, want 4", compute.Value())
	}
}

func TestComputeCell2(t *testing.T) {
	r := New()
	a := r.CreateInput(1)
	b := r.CreateInput(2)
	sum := r.CreateCompute2(a, b, func(x, y int) int { return x + y })
	if sum.Value() != 3 {
		t.Fatalf("sum.Value() = %d, want 3", sum.Value())
	}
}

func TestComputeCell2Updates(t *testing.T) {
	r := New()
	a := r.CreateInput(1)
	b := r.CreateInput(2)
	sum := r.CreateCompute2(a, b, func(x, y int) int { return x + y })
	a.SetValue(10)
	if sum.Value() != 12 {
		t.Fatalf("sum.Value() = %d, want 12", sum.Value())
	}
}

func TestCallbackFires(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	compute := r.CreateCompute1(input, func(v int) int { return v + 1 })

	var observed []int
	compute.AddCallback(func(v int) {
		observed = append(observed, v)
	})

	input.SetValue(3)
	if len(observed) != 1 || observed[0] != 4 {
		t.Fatalf("observed = %v, want [4]", observed)
	}
}

func TestCallbackNotFiredIfNoChange(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	compute := r.CreateCompute1(input, func(v int) int {
		if v < 3 {
			return 1
		}
		return 2
	})

	callCount := 0
	compute.AddCallback(func(v int) {
		callCount++
	})

	input.SetValue(2) // compute still returns 1
	if callCount != 0 {
		t.Fatalf("callback called %d times, want 0", callCount)
	}
}

func TestRemoveCallback(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	compute := r.CreateCompute1(input, func(v int) int { return v + 1 })

	callCount := 0
	remove := compute.AddCallback(func(v int) {
		callCount++
	})

	input.SetValue(2)
	remove()
	input.SetValue(3)

	if callCount != 1 {
		t.Fatalf("callback called %d times, want 1", callCount)
	}
}

func TestChainedComputeCells(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	times2 := r.CreateCompute1(input, func(v int) int { return v * 2 })
	times30 := r.CreateCompute1(input, func(v int) int { return v * 30 })
	sum := r.CreateCompute2(times2, times30, func(x, y int) int { return x + y })

	if sum.Value() != 32 {
		t.Fatalf("sum.Value() = %d, want 32", sum.Value())
	}

	input.SetValue(3)
	if sum.Value() != 96 {
		t.Fatalf("sum.Value() = %d, want 96", sum.Value())
	}
}

func TestCallbackOnlyFiresOncePerPropagation(t *testing.T) {
	r := New()
	input := r.CreateInput(1)
	times2 := r.CreateCompute1(input, func(v int) int { return v * 2 })
	times30 := r.CreateCompute1(input, func(v int) int { return v * 30 })
	sum := r.CreateCompute2(times2, times30, func(x, y int) int { return x + y })

	callCount := 0
	sum.AddCallback(func(v int) {
		callCount++
	})

	input.SetValue(4)
	if callCount != 1 {
		t.Fatalf("callback called %d times, want 1", callCount)
	}
}
