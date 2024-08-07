package core

import "github.ibm.com/tantawi/inferno/pkg/config"

type Optimizer struct {
	Spec *config.OptimizerSpec
}

func NewOptimizerFromSpec(spec *config.OptimizerSpec) *Optimizer {
	return &Optimizer{
		Spec: spec,
	}
}

func (o *Optimizer) Optimize(system *System) {
	if o.Spec == nil {
		return
	}
	solver := NewSolver(o.Spec.Unlimited)
	solver.Solve(system)
	system.AllocateByType()
}
