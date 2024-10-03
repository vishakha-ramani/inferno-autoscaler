package core

import (
	"bytes"
	"fmt"
	"time"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

type Optimizer struct {
	spec             *config.OptimizerSpec
	solver           *Solver
	solutionTimeMsec int64
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
	o.solver = NewSolver(o.spec.Unlimited, o.spec.Heterogeneous,
		o.spec.MILPSolver, o.spec.UseCplex)

	startTime := time.Now()
	o.solver.Solve(system)
	endTime := time.Now()
	o.solutionTimeMsec = endTime.Sub(startTime).Milliseconds()

	system.AllocateByType()
}

func (o *Optimizer) GetSolutionTimeMsec() int64 {
	return o.solutionTimeMsec
}

func (o *Optimizer) String() string {
	var b bytes.Buffer
	if o.solver != nil {
		b.WriteString(o.solver.String())
	}
	fmt.Fprintf(&b, "Solution time: %d msec\n", o.solutionTimeMsec)
	return b.String()
}
