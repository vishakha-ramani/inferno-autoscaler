package core

import "fmt"

type Accelerator struct {
	Name string
	Spec *AcceleratorSpec

	slopeLow  float32
	slopeHigh float32
}

type AcceleratorSpec struct {
	Type          string    `json:"type"`
	Multiplicity  int       `json:"multiplicity"`
	MemSize       int       `json:"memSize"` // GB
	MemBW         int       `json:"memBW"`   // GB/sec
	RelativeSpeed int       `json:"relativeSpeed"`
	Power         PowerSpec `json:"power"`
	Cost          float32   `json:"cost"` // cents/hr
}

type PowerSpec struct {
	Idle     int     `json:"idle"`
	Full     int     `json:"full"`
	MidPower int     `json:"midPower"`
	MidUtil  float32 `json:"midUtil"`
}

func (g *Accelerator) GetType() string {
	return g.Spec.Type
}

func NewAcceleratorFromSpec(name string, spec *AcceleratorSpec) *Accelerator {
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

func (g *Accelerator) String() string {
	return fmt.Sprintf("Accelerator: name=%s; type=%s; multiplicity=%d; memSize=%d; memBW=%d; relativeSpeed=%d; cost=%v; power={%d,%d,%d@%v}",
		g.Name, g.Spec.Type, g.Spec.Multiplicity, g.Spec.MemSize, g.Spec.MemBW, g.Spec.RelativeSpeed, g.Spec.Cost,
		g.Spec.Power.Idle, g.Spec.Power.Full, g.Spec.Power.MidPower, g.Spec.Power.MidUtil)
}
