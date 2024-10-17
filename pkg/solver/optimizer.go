package solver

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

// Create optimizer from spec
func NewOptimizerFromSpec(spec *config.OptimizerSpec) *Optimizer {
	return &Optimizer{
		spec: spec,
	}
}

func (o *Optimizer) Optimize() {
	if o.spec == nil {
		return
	}
	o.solver = NewSolver(o.spec)

	startTime := time.Now()
	o.solver.Solve()
	endTime := time.Now()
	o.solutionTimeMsec = endTime.Sub(startTime).Milliseconds()
}

func (o *Optimizer) SolutionTimeMsec() int64 {
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
