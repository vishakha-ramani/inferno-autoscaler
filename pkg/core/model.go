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

	// number of accelerator instances needed to fit a model on a given accelerator
	numInstances map[string]int
}

func NewModelFromSpec(spec *config.ModelSpec) *Model {
	return &Model{
		spec:         spec,
		perfData:     make(map[string]*config.ModelAcceleratorPerfData),
		numInstances: make(map[string]int),
	}
}

// Calculate basic parameters
func (m *Model) Calculate(accelerators map[string]*Accelerator) {
	for gName := range m.perfData {
		if g, exists := accelerators[gName]; exists {
			m.numInstances[gName] = int(math.Ceil(float64(m.spec.MemSize) / float64(g.spec.MemSize)))
		}
	}
}

func (m *Model) GetName() string {
	return m.spec.Name
}

func (m *Model) GetNumInstances(acceleratorName string) int {
	return m.numInstances[acceleratorName]
}

func (m *Model) String() string {
	return fmt.Sprintf("Model: name=%s; memSize=%d",
		m.spec.Name, m.spec.MemSize)
}
