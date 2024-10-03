package solver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/core"
)

type Optimizer struct {
	spec             *config.OptimizerSpec
	solver           *Solver
	solutionTimeMsec int64
}

// Create optimizer from spec
func NewOptimizerFromSpec(byteValue []byte) (*Optimizer, error) {
	var d config.OptimizerData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return nil, err
	}
	o := &Optimizer{
		spec: &d.Spec,
	}
	return o, nil
}

func (o *Optimizer) Optimize(system *core.System) {
	if o.spec == nil {
		return
	}
	o.solver = NewSolver(o.spec)

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
