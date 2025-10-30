package tuner

import (
	"bytes"
	"fmt"

	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/analyzer"
	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	kalman "github.com/llm-inferno/kalman-filter/pkg/core"
	"gonum.org/v1/gonum/mat"
)

type Tuner struct {
	configurator *Configurator
	filter       *kalman.ExtendedKalmanFilter
	env          *Environment
}

func NewTuner(configData *TunerConfigData, env *Environment) (tuner *Tuner, err error) {
	var c *Configurator
	var f *kalman.ExtendedKalmanFilter

	t := &Tuner{
		env: env,
	}

	// create configurator
	if c, err = NewConfigurator(configData); err != nil {
		return nil, err
	}

	//create filter
	f, err = kalman.NewExtendedKalmanFilter(c.NumStates(), c.NumObservations(), c.X0, c.P)
	if err != nil {
		return nil, err
	}
	if err := f.SetQ(c.Q); err != nil {
		return nil, err
	}
	if err := f.SetR(c.R); err != nil {
		return nil, err
	}
	if err := f.SetfF(c.fFunc); err != nil {
		return nil, err
	}
	if c.Xbounded {
		if err := f.SetStateLimiter(c.Xmin, c.Xmax); err != nil {
			return nil, err
		}
	}
	if err := f.SethH(t.makeObservationFunc()); err != nil {
		return nil, err
	}

	t.configurator = c
	t.filter = f

	return t, nil
}

func (t *Tuner) Run() error {
	Q := t.filter.Q
	if err := t.filter.Predict(Q); err != nil {
		fmt.Println(err)
		return err
	}

	// correct
	Z := t.env.GetObservations()
	if err := t.filter.Update(Z, t.configurator.R); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func (t *Tuner) X() *mat.VecDense {
	return t.filter.State()
}

func (t *Tuner) Innovation() *mat.VecDense {
	return t.filter.Innovation()
}

func (t *Tuner) P() *mat.Dense {
	return t.filter.P
}

func (t *Tuner) S() *mat.Dense {
	return t.filter.S
}

func (t *Tuner) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Tuner: \n")
	fmt.Fprintf(&b, "%v\n", t.configurator)
	fmt.Fprintf(&b, "%v\n", t.env)
	return b.String()
}

func (t *Tuner) UpdateEnvironment(envt *Environment) {
	t.env = envt
}

func (t *Tuner) GetParms() *mat.VecDense {
	// TODO: intelligent state return
	return t.X()
}

func (t *Tuner) makeObservationFunc() func(x *mat.VecDense) *mat.VecDense {
	return func(x *mat.VecDense) *mat.VecDense {
		// TODO: write for the case where max batch size is not available.
		// See for example:
		// 	var N int
		// if server.maxBatchSize > 0 {
		// 	N = server.maxBatchSize
		// } else {
		// 	N = max(perf.MaxBatchSize*perf.AtTokens/K, 1)
		// }
		N := t.env.MaxBatchSize
		maxQueue := N * config.MaxQueueToBatchRatio
		qConfig := &analyzer.Configuration{
			MaxBatchSize: N,
			MaxQueueSize: maxQueue,
			ServiceParms: &analyzer.ServiceParms{
				Prefill: &analyzer.PrefillParms{
					Gamma: float32(x.AtVec(3)),
					Delta: float32(x.AtVec(2)),
				},
				Decode: &analyzer.DecodeParms{
					Alpha: float32(x.AtVec(0)),
					Beta:  float32(x.AtVec(1)),
				},
			},
		}
		requestData := &analyzer.RequestSize{
			AvgInputTokens:  t.env.AvgInputToks,
			AvgOutputTokens: t.env.AvgOutputToks,
		}

		qa, err := analyzer.NewQueueAnalyzer(qConfig, requestData)
		if err != nil {
			fmt.Println(err)
			return mat.NewVecDense(t.configurator.nX, nil)
		}

		lambda := t.env.Lambda / 60 // convert to req per sec
		metrics, err := qa.Analyze(lambda)
		if err != nil {
			fmt.Println(err)
			return mat.NewVecDense(t.configurator.nX, nil)
		}

		ttft := float64(metrics.AvgWaitTime + metrics.AvgPrefillTime)
		itl := float64(metrics.AvgTokenTime)

		return mat.NewVecDense(2, []float64{ttft, itl})
	}
}
