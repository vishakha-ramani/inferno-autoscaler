package core

import (
	"strings"
	"testing"

	"github.com/llm-d-incubation/workload-variant-autoscaler/hack/inferno/pkg/config"
)

// Helper function to create a basic allocation for testing
func createTestAllocation() *Allocation {
	return &Allocation{
		accelerator:           "test-gpu",
		numReplicas:           2,
		batchSize:             8,
		cost:                  100.0,
		value:                 100.0,
		itl:                   10.5,
		ttft:                  25.0,
		rho:                   0.7,
		maxArrvRatePerReplica: 0.05,
	}
}

func TestAllocation_Getters(t *testing.T) {
	alloc := createTestAllocation()

	tests := []struct {
		name     string
		getter   func() any
		expected any
	}{
		{
			name:     "Accelerator",
			getter:   func() any { return alloc.Accelerator() },
			expected: "test-gpu",
		},
		{
			name:     "NumReplicas",
			getter:   func() any { return alloc.NumReplicas() },
			expected: 2,
		},
		{
			name:     "MaxBatchSize",
			getter:   func() any { return alloc.MaxBatchSize() },
			expected: 8,
		},
		{
			name:     "Cost",
			getter:   func() any { return alloc.Cost() },
			expected: float32(100.0),
		},
		{
			name:     "Value",
			getter:   func() any { return alloc.Value() },
			expected: float32(100.0),
		},
		{
			name:     "MaxArrvRatePerReplica",
			getter:   func() any { return alloc.MaxArrvRatePerReplica() },
			expected: float32(0.05),
		},
		{
			name:     "MaxRPM",
			getter:   func() any { return alloc.MaxRPM() },
			expected: float32(3000.0), // 0.05 * 1000 * 60
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.getter()
			if got != tt.expected {
				t.Errorf("%s() = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestAllocation_Setters(t *testing.T) {
	alloc := createTestAllocation()

	tests := []struct {
		name     string
		setter   func()
		getter   func() any
		expected any
	}{
		{
			name:     "SetNumReplicas",
			setter:   func() { alloc.SetNumReplicas(5) },
			getter:   func() any { return alloc.NumReplicas() },
			expected: 5,
		},
		{
			name:     "SetMaxBatchSize",
			setter:   func() { alloc.SetMaxBatchSize(16) },
			getter:   func() any { return alloc.MaxBatchSize() },
			expected: 16,
		},
		{
			name:     "SetCost",
			setter:   func() { alloc.SetCost(250.0) },
			getter:   func() any { return alloc.Cost() },
			expected: float32(250.0),
		},
		{
			name:     "SetValue",
			setter:   func() { alloc.SetValue(300.0) },
			getter:   func() any { return alloc.Value() },
			expected: float32(300.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setter()
			got := tt.getter()
			if got != tt.expected {
				t.Errorf("After %s, got %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestAllocation_Saturated(t *testing.T) {
	alloc := createTestAllocation()
	// MaxRPM = 0.05 * 1000 * 60 * 2 replicas = 6000

	tests := []struct {
		name      string
		totalRate float32
		want      bool
	}{
		{
			name:      "below saturation",
			totalRate: 5000.0,
			want:      false,
		},
		{
			name:      "at saturation",
			totalRate: 6000.0,
			want:      false,
		},
		{
			name:      "above saturation",
			totalRate: 7000.0,
			want:      true,
		},
		{
			name:      "zero rate",
			totalRate: 0.0,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alloc.Saturated(tt.totalRate)
			if got != tt.want {
				t.Errorf("Allocation.Saturated(%v) = %v, want %v", tt.totalRate, got, tt.want)
			}
		})
	}
}

func TestAllocation_TransitionPenalty(t *testing.T) {
	allocA := &Allocation{
		accelerator: "gpu-a",
		numReplicas: 2,
		cost:        100.0,
	}

	tests := []struct {
		name   string
		allocB *Allocation
		want   float32
	}{
		{
			name: "same accelerator same replicas",
			allocB: &Allocation{
				accelerator: "gpu-a",
				numReplicas: 2,
				cost:        100.0,
			},
			want: 0.0,
		},
		{
			name: "same accelerator different replicas",
			allocB: &Allocation{
				accelerator: "gpu-a",
				numReplicas: 3,
				cost:        150.0,
			},
			want: 50.0, // cost difference
		},
		{
			name: "different accelerator",
			allocB: &Allocation{
				accelerator: "gpu-b",
				numReplicas: 2,
				cost:        120.0,
			},
			want: config.AccelPenaltyFactor*(100.0+120.0) + (120.0 - 100.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allocA.TransitionPenalty(tt.allocB)
			if got != tt.want {
				t.Errorf("TransitionPenalty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllocation_Clone(t *testing.T) {
	original := createTestAllocation()
	cloned := original.Clone()

	// Verify all fields are copied
	if cloned.accelerator != original.accelerator {
		t.Errorf("Clone accelerator = %v, want %v", cloned.accelerator, original.accelerator)
	}
	if cloned.numReplicas != original.numReplicas {
		t.Errorf("Clone numReplicas = %v, want %v", cloned.numReplicas, original.numReplicas)
	}
	if cloned.batchSize != original.batchSize {
		t.Errorf("Clone batchSize = %v, want %v", cloned.batchSize, original.batchSize)
	}
	if cloned.cost != original.cost {
		t.Errorf("Clone cost = %v, want %v", cloned.cost, original.cost)
	}
	if cloned.value != original.value {
		t.Errorf("Clone value = %v, want %v", cloned.value, original.value)
	}

	// Verify the cloned copy is a different reference
	if cloned == original {
		t.Error("Clone returned same reference instead of new reference")
	}
	// Verify modifying cloned copy doesn't affect the original copy
	cloned.SetNumReplicas(5)
	if original.NumReplicas() == 5 {
		t.Error("Modifying clone affected original")
	}
}

func TestAllocation_AllocationData(t *testing.T) {
	alloc := createTestAllocation()
	data := alloc.AllocationData()

	if data.Accelerator != alloc.accelerator {
		t.Errorf("AllocationData.Accelerator = %v, want %v", data.Accelerator, alloc.accelerator)
	}
	if data.NumReplicas != alloc.numReplicas {
		t.Errorf("AllocationData.NumReplicas = %v, want %v", data.NumReplicas, alloc.numReplicas)
	}
	if data.MaxBatch != alloc.batchSize {
		t.Errorf("AllocationData.MaxBatch = %v, want %v", data.MaxBatch, alloc.batchSize)
	}
	if data.Cost != alloc.cost {
		t.Errorf("AllocationData.Cost = %v, want %v", data.Cost, alloc.cost)
	}
	if data.ITLAverage != alloc.itl {
		t.Errorf("AllocationData.ITLAverage = %v, want %v", data.ITLAverage, alloc.itl)
	}
	if data.TTFTAverage != alloc.ttft {
		t.Errorf("AllocationData.TTFTAverage = %v, want %v", data.TTFTAverage, alloc.ttft)
	}
}

func TestAllocationFromData(t *testing.T) {
	data := &config.AllocationData{
		Accelerator: "test-gpu",
		NumReplicas: 3,
		MaxBatch:    16,
		Cost:        200.0,
		ITLAverage:  15.5,
		TTFTAverage: 30.0,
	}

	alloc := AllocationFromData(data)

	if alloc.accelerator != data.Accelerator {
		t.Errorf("AllocationFromData accelerator = %v, want %v", alloc.accelerator, data.Accelerator)
	}
	if alloc.numReplicas != data.NumReplicas {
		t.Errorf("AllocationFromData numReplicas = %v, want %v", alloc.numReplicas, data.NumReplicas)
	}
	if alloc.batchSize != data.MaxBatch {
		t.Errorf("AllocationFromData batchSize = %v, want %v", alloc.batchSize, data.MaxBatch)
	}
	if alloc.cost != data.Cost {
		t.Errorf("AllocationFromData cost = %v, want %v", alloc.cost, data.Cost)
	}
	if alloc.itl != data.ITLAverage {
		t.Errorf("AllocationFromData itl = %v, want %v", alloc.itl, data.ITLAverage)
	}
	if alloc.ttft != data.TTFTAverage {
		t.Errorf("AllocationFromData ttft = %v, want %v", alloc.ttft, data.TTFTAverage)
	}
}

func TestAllocation_String(t *testing.T) {
	alloc := createTestAllocation()
	str := alloc.String()

	// Verify string contains key information
	expectedSubstrings := []string{
		"test-gpu",   // accelerator name
		"numRep=2",   // num replicas
		"maxBatch=8", // batch size
		"cost=100",   // cost
		"val=100",    // value
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(str, substr) {
			t.Errorf("String() = %v, should contain %v", str, substr)
		}
	}
}

func TestCreateAllocationDiff(t *testing.T) {
	tests := []struct {
		name    string
		a       *Allocation
		b       *Allocation
		wantNil bool
	}{
		{
			name:    "both nil",
			a:       nil,
			b:       nil,
			wantNil: true,
		},
		{
			name:    "a nil, b not nil",
			a:       nil,
			b:       createTestAllocation(),
			wantNil: false,
		},
		{
			name:    "a not nil, b nil",
			a:       createTestAllocation(),
			b:       nil,
			wantNil: false,
		},
		{
			name:    "both not nil",
			a:       createTestAllocation(),
			b:       createTestAllocation(),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := CreateAllocationDiff(tt.a, tt.b)
			if (diff == nil) != tt.wantNil {
				t.Errorf("CreateAllocationDiff() = %v, wantNil %v", diff, tt.wantNil)
			}
		})
	}
}

func TestAllocationDiff_Content(t *testing.T) {
	allocA := &Allocation{
		accelerator: "gpu-a",
		numReplicas: 2,
		cost:        100.0,
	}

	allocB := &Allocation{
		accelerator: "gpu-b",
		numReplicas: 3,
		cost:        150.0,
	}

	diff := CreateAllocationDiff(allocA, allocB)

	if diff.oldAccelerator != "gpu-a" {
		t.Errorf("oldAccelerator = %v, want gpu-a", diff.oldAccelerator)
	}
	if diff.newAccelerator != "gpu-b" {
		t.Errorf("newAccelerator = %v, want gpu-b", diff.newAccelerator)
	}
	if diff.oldNumReplicas != 2 {
		t.Errorf("oldNumReplicas = %v, want 2", diff.oldNumReplicas)
	}
	if diff.newNumReplicas != 3 {
		t.Errorf("newNumReplicas = %v, want 3", diff.newNumReplicas)
	}
	if diff.costDiff != 50.0 {
		t.Errorf("costDiff = %v, want 50.0", diff.costDiff)
	}
}

func TestAllocationDiff_String(t *testing.T) {
	allocA := &Allocation{
		accelerator: "gpu-a",
		numReplicas: 2,
		cost:        100.0,
	}

	allocB := &Allocation{
		accelerator: "gpu-b",
		numReplicas: 3,
		cost:        150.0,
	}

	diff := CreateAllocationDiff(allocA, allocB)
	str := diff.String()

	expectedSubstrings := []string{
		"gpu-a -> gpu-b",
		"2 -> 3",
		"50",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(str, substr) {
			t.Errorf("String() = %v, should contain %v", str, substr)
		}
	}
}

func TestAllocationDiff_NilHandling(t *testing.T) {
	tests := []struct {
		name            string
		a               *Allocation
		b               *Allocation
		wantOldAcc      string
		wantNewAcc      string
		wantOldReplicas int
		wantNewReplicas int
	}{
		{
			name:            "nil to allocation",
			a:               nil,
			b:               createTestAllocation(),
			wantOldAcc:      "none",
			wantNewAcc:      "test-gpu",
			wantOldReplicas: 0,
			wantNewReplicas: 2,
		},
		{
			name:            "allocation to nil",
			a:               createTestAllocation(),
			b:               nil,
			wantOldAcc:      "test-gpu",
			wantNewAcc:      "none",
			wantOldReplicas: 2,
			wantNewReplicas: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := CreateAllocationDiff(tt.a, tt.b)
			if diff.oldAccelerator != tt.wantOldAcc {
				t.Errorf("oldAccelerator = %v, want %v", diff.oldAccelerator, tt.wantOldAcc)
			}
			if diff.newAccelerator != tt.wantNewAcc {
				t.Errorf("newAccelerator = %v, want %v", diff.newAccelerator, tt.wantNewAcc)
			}
			if diff.oldNumReplicas != tt.wantOldReplicas {
				t.Errorf("oldNumReplicas = %v, want %v", diff.oldNumReplicas, tt.wantOldReplicas)
			}
			if diff.newNumReplicas != tt.wantNewReplicas {
				t.Errorf("newNumReplicas = %v, want %v", diff.newNumReplicas, tt.wantNewReplicas)
			}
		})
	}
}
