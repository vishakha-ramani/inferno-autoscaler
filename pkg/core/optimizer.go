package core

import (
	"bytes"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

type Optimizer struct {
	spec *config.OptimizerSpec

	solver *Solver
}

func NewOptimizerFromSpec(spec *config.OptimizerSpec) *Optimizer {
	return &Optimizer{
		spec: spec,
	}
}

func (o *Optimizer) Optimize(system *System) {
	if o.spec == nil {
		return
	}
	o.solver = NewSolver(o.spec.Unlimited)
	o.solver.Solve(system)
	system.AllocateByType()
}

func (o *Optimizer) String() string {
	var b bytes.Buffer
	if o.solver != nil {
		b.WriteString(o.solver.String())
	}
	return b.String()
}
