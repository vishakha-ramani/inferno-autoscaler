package core

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// An accelerator used in an inference server
//   - full GPU unit
//   - multiple GPU units
type Accelerator struct {
	name string
	spec *config.AcceleratorSpec

	slopeLow  float32 // power profile slope at low utilization
	slopeHigh float32 // power profile slope at high utilization
}

func NewAcceleratorFromSpec(name string, spec *config.AcceleratorSpec) *Accelerator {
	return &Accelerator{
		name: name,
		spec: spec,
	}
}

// Calculate basic parameters
func (g *Accelerator) Calculate() {
	g.slopeLow = float32(g.spec.Power.MidPower-g.spec.Power.Idle) / g.spec.Power.MidUtil
	g.slopeHigh = float32(g.spec.Power.Full-g.spec.Power.MidPower) / (1 - g.spec.Power.MidUtil)
}

// Evaluate power consumption at a given utilization
func (g *Accelerator) Power(util float32) float32 {
	if util <= g.spec.Power.MidUtil {
		return float32(g.spec.Power.Idle) + g.slopeLow*util
	} else {
		return float32(g.spec.Power.MidPower) + g.slopeHigh*(util-g.spec.Power.MidUtil)
	}
}

func (g *Accelerator) GetType() string {
	return g.spec.Type
}

func (g *Accelerator) GetSpec() *config.AcceleratorSpec {
	return g.spec
}

func (g *Accelerator) String() string {
	return fmt.Sprintf("Accelerator: name=%s; type=%s; multiplicity=%d; memSize=%d; memBW=%d; cost=%v; power={ %d, %d, %d @ %v }",
		g.name, g.spec.Type, g.spec.Multiplicity, g.spec.MemSize, g.spec.MemBW, g.spec.Cost,
		g.spec.Power.Idle, g.spec.Power.Full, g.spec.Power.MidPower, g.spec.Power.MidUtil)
}
