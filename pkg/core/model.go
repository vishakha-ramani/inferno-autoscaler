package core

import (
	"fmt"
	"math"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

type Model struct {
	Spec *config.ModelSpec

	perfData map[string]*config.ModelPerfData

	// number of accelerator units needed to fit a model on a given accelerator
	numUnits map[string]int
}

func NewModelFromSpec(spec *config.ModelSpec) *Model {
	perfData := make(map[string]*config.ModelPerfData)
	for _, pf := range spec.AccSpec {
		perfData[pf.Name] = &pf
	}
	return &Model{
		Spec:     spec,
		perfData: perfData,
		numUnits: make(map[string]int),
	}
}

// Calculate basic parameters
func (m *Model) Calculate(accelerators map[string]*Accelerator) {
	for k, v := range accelerators {
		m.numUnits[k] = int(math.Ceil(float64(m.Spec.MemSize) / float64(v.Spec.MemSize)))
	}
}

func (m *Model) String() string {
	return fmt.Sprintf("Model: name=%s; memSize=%d; numUnits= %v",
		m.Spec.Name, m.Spec.MemSize, m.numUnits)
}
