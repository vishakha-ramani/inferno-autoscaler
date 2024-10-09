package core

import (
	"fmt"
	"math"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// An inference model
type Model struct {
	name string
	spec *config.ModelSpec

	// model performance data for specified accelerators
	perfData map[string]*config.ModelAcceleratorPerfData

	// number of accelerator instances needed to fit a model on a given accelerator
	numInstances map[string]int
}

func NewModelFromSpec(spec *config.ModelSpec) *Model {
	return &Model{
		name:         spec.Name,
		spec:         spec,
		perfData:     make(map[string]*config.ModelAcceleratorPerfData),
		numInstances: make(map[string]int),
	}
}

// Calculate basic parameters
func (m *Model) Calculate(accelerators map[string]*Accelerator) {
	for gName := range m.perfData {
		if g, exists := accelerators[gName]; exists {
			m.numInstances[gName] = int(math.Ceil(float64(m.spec.MemSize) / float64(g.MemSize())))
		}
	}
}

func (m *Model) Name() string {
	return m.name
}

func (m *Model) Spec() *config.ModelSpec {
	return m.spec
}

func (m *Model) NumInstances(acceleratorName string) int {
	return m.numInstances[acceleratorName]
}

func (m *Model) PerfData(acceleratorName string) *config.ModelAcceleratorPerfData {
	return m.perfData[acceleratorName]
}

func (m *Model) AddPerfDataFromSpec(spec *config.ModelAcceleratorPerfData) {
	if spec.Name == m.name {
		m.perfData[spec.Acc] = spec
	}
}

func (m *Model) RemovePerfData(accName string) {
	delete(m.perfData, accName)
}

func (m *Model) String() string {
	return fmt.Sprintf("Model: name=%s; memSize=%d",
		m.spec.Name, m.spec.MemSize)
}
