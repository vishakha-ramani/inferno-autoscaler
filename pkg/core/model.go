package core

import (
	"fmt"
	"math"
)

type Model struct {
	Spec *ModelSpec

	// number of accelerator units needed to fit a model on a given accelerator
	numUnits map[string]int

	// multiplier of token service time (inter-token) on a given accelerator, relative to profiled accelarator
	serviceTimeMultiplier map[string]float32

	// multiplier of max batch size on a given accelerator, relative to profiled accelerator
	MaxBatchSizeMultiplier map[string]float32
}

type ModelSpec struct {
	Name         string  `json:"name"`
	MemSize      int     `json:"memSize"` // GB
	MaxBatchSize int     `json:"maxBatchSize"`
	AtTokens     int     `json:"atTokens"`
	Alpha        float32 `json:"alpha"`
	Beta         float32 `json:"beta"`
	Profiled     string  `json:"profiled"`
}

func NewModelFromSpec(spec *ModelSpec) *Model {
	return &Model{
		Spec:                   spec,
		numUnits:               make(map[string]int),
		serviceTimeMultiplier:  make(map[string]float32),
		MaxBatchSizeMultiplier: make(map[string]float32),
	}
}

// Calculate basic parameters
func (m *Model) Calculate(accelerators map[string]*Accelerator) {
	for k, v := range accelerators {
		m.numUnits[k] = int(math.Ceil(float64(m.Spec.MemSize) / float64(v.Spec.MemSize)))
		m.serviceTimeMultiplier[k] = float32(accelerators[m.Spec.Profiled].Spec.RelativeSpeed) / float32(v.Spec.RelativeSpeed)
		m.MaxBatchSizeMultiplier[k] = float32(v.Spec.MemSize) / float32(accelerators[m.Spec.Profiled].Spec.MemSize)
	}
}

func (m *Model) String() string {
	return fmt.Sprintf("Model: name=%s; memSize=%d; maxBatchSize=%d; atTokens=%d; alpha=%v, beta=%v; profiled=%s; numUnits= %v; serviceTimeMultiplier=%v",
		m.Spec.Name, m.Spec.MemSize, m.Spec.MaxBatchSize, m.Spec.AtTokens, m.Spec.Alpha, m.Spec.Beta, m.Spec.Profiled, m.numUnits, m.serviceTimeMultiplier)
}
