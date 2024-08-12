package core

import (
	"fmt"
	"math"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// An inference model
type Model struct {
	spec *config.ModelSpec

	// model performance data for specified accelerators
	perfData map[string]*config.ModelAcceleratorPerfData

	// number of accelerator units needed to fit a model on a given accelerator
	numUnits map[string]int
}

func NewModelFromSpec(spec *config.ModelSpec) *Model {
	perfData := make(map[string]*config.ModelAcceleratorPerfData)
	for _, pf := range spec.PerfData {
		perfData[pf.Name] = &pf
	}
	return &Model{
		spec:     spec,
		perfData: perfData,
		numUnits: make(map[string]int),
	}
}

// Calculate basic parameters
func (m *Model) Calculate(accelerators map[string]*Accelerator) {
	for gName := range m.perfData {
		if g, exists := accelerators[gName]; exists {
			m.numUnits[gName] = int(math.Ceil(float64(m.spec.MemSize) / float64(g.spec.MemSize)))
		}
	}
}

func (m *Model) GetName() string {
	return m.spec.Name
}

func (m *Model) String() string {
	return fmt.Sprintf("Model: name=%s; memSize=%d; numUnits= %v",
		m.spec.Name, m.spec.MemSize, m.numUnits)
}
