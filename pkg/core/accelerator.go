package core

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

type Accelerator struct {
	Name string
	Spec *config.AcceleratorSpec

	slopeLow  float32
	slopeHigh float32
}

func NewAcceleratorFromSpec(name string, spec *config.AcceleratorSpec) *Accelerator {
	return &Accelerator{
		Name: name,
		Spec: spec,
	}
}

// Calculate basic parameters
func (g *Accelerator) Calculate() {
	g.slopeLow = float32(g.Spec.Power.MidPower-g.Spec.Power.Idle) / g.Spec.Power.MidUtil
	g.slopeHigh = float32(g.Spec.Power.Full-g.Spec.Power.MidPower) / (1 - g.Spec.Power.MidUtil)
}

// Evaluate power consumption at a given utilization
func (g *Accelerator) Power(util float32) float32 {
	if util <= g.Spec.Power.MidUtil {
		return float32(g.Spec.Power.Idle) + g.slopeLow*util
	} else {
		return float32(g.Spec.Power.MidPower) + g.slopeHigh*(util-g.Spec.Power.MidUtil)
	}
}

func (g *Accelerator) GetType() string {
	return g.Spec.Type
}

func (g *Accelerator) String() string {
	return fmt.Sprintf("Accelerator: name=%s; type=%s; multiplicity=%d; memSize=%d; memBW=%d; cost=%v; power={%d,%d,%d@%v}",
		g.Name, g.Spec.Type, g.Spec.Multiplicity, g.Spec.MemSize, g.Spec.MemBW, g.Spec.Cost,
		g.Spec.Power.Idle, g.Spec.Power.Full, g.Spec.Power.MidPower, g.Spec.Power.MidUtil)
}
