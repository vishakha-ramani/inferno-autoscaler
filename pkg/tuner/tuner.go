package tuner

import (
	"bytes"
	"fmt"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/constants"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/analyzer"
	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	kalman "github.com/llm-inferno/kalman-filter/pkg/core"
	"gonum.org/v1/gonum/mat"
)

// State vector indices for model parameters
const (
	StateIndexAlpha = 0 // Decode base parameter
	StateIndexBeta  = 1 // Decode slope parameter
	StateIndexGamma = 2 // Prefill base parameter
	StateIndexDelta = 3 // Prefill slope parameter
)

type Tuner struct {
	configurator *Configurator
	filter       *kalman.ExtendedKalmanFilter
	env          *Environment
}

// TunedResults holds the results of parameter tuning
type TunedResults struct {
	ServiceParms *analyzer.ServiceParms
	Innovation   *mat.VecDense
	Covariance   *mat.Dense
	NIS          float64
}

func NewTuner(configData *TunerConfigData, env *Environment) (tuner *Tuner, err error) {
	var c *Configurator
	var f *kalman.ExtendedKalmanFilter

	// Validate inputs
	if env == nil {
		return nil, fmt.Errorf("environment cannot be nil")
	}
	if !env.Valid() {
		return nil, fmt.Errorf("invalid environment: %v", env)
	}

	// create configurator
	if c, err = NewConfigurator(configData); err != nil {
		return nil, fmt.Errorf("error on configurator creation: %v", err)
	}

	// create filter
	f, err = kalman.NewExtendedKalmanFilter(c.NumStates(), c.NumObservations(), c.X0, c.P)
	if err != nil {
		return nil, fmt.Errorf("error on filter creation: %v", err)
	}
	if err := f.SetQ(c.Q); err != nil {
		return nil, fmt.Errorf("error on setting Q: %v", err)
	}
	if err := f.SetR(c.R); err != nil {
		return nil, fmt.Errorf("error on setting R: %v", err)
	}
	if err := f.SetfF(c.fFunc); err != nil {
		return nil, fmt.Errorf("error on setting fFunc: %v", err)
	}
	if c.Xbounded {
		if err := f.SetStateLimiter(c.Xmin, c.Xmax); err != nil {
			return nil, fmt.Errorf("error on setting state limiter: %v", err)
		}
	}

	// create tuner
	t := &Tuner{
		env:          env,
		configurator: c,
		filter:       f,
	}

	// assign observation function to filter
	if err := f.SethH(t.makeObservationFunc()); err != nil {
		return nil, fmt.Errorf("error on setting observation function: %v", err)
	}

	return t, nil
}

func (t *Tuner) Run() (tunedResults *TunedResults, err error) {
	// validate environment before running
	if !t.env.Valid() {
		return nil, fmt.Errorf("cannot run tuner with invalid environment: %v", t.env)
	}

	// create a stasher and stash the current X and P
	stasher := NewStasher(t.filter)
	if stasher == nil {
		return nil, fmt.Errorf("failed to create stasher")
	}
	if err := stasher.Stash(); err != nil {
		return nil, fmt.Errorf("failed to stash filter state: %w", err)
	}

	// prediction
	Q := t.filter.Q
	if err := t.filter.Predict(Q); err != nil {
		return nil, fmt.Errorf("failed to predict: %w", err)
	}

	// update
	Z := t.env.GetObservations()
	if err := t.filter.Update(Z, t.configurator.R); err != nil {
		return nil, fmt.Errorf("failed to update filter: %w", err)
	}

	// Extract tuned parameters
	tunedResults, err = t.extractTunedResults()
	if err != nil {
		return nil, fmt.Errorf("failed to extract tuned params: %w", err)
	}

	// check validity of tunedResults
	if err := t.validateTunedResults(tunedResults); err != nil {
		// unstash to return to previous filter state
		if err := stasher.UnStash(); err != nil {
			return nil, fmt.Errorf("failed to unstash filter state: %w", err)
		}
		return nil, fmt.Errorf("tuned results validation failed: %w", err)
	}

	return tunedResults, nil
}

func (t *Tuner) X() *mat.VecDense {
	return t.filter.State()
}

func (t *Tuner) Y() *mat.VecDense {
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

func (t *Tuner) UpdateEnvironment(env *Environment) error {
	if env == nil {
		return fmt.Errorf("environment cannot be nil")
	}
	if !env.Valid() {
		return fmt.Errorf("invalid environment: %v", env)
	}
	t.env = env
	return nil
}

func (t *Tuner) GetParms() *mat.VecDense {
	// TODO: intelligent state return
	return t.X()
}

func (t *Tuner) GetEnvironment() *Environment {
	return t.env
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
					Gamma: float32(x.AtVec(StateIndexGamma)),
					Delta: float32(x.AtVec(StateIndexDelta)),
				},
				Decode: &analyzer.DecodeParms{
					Alpha: float32(x.AtVec(StateIndexAlpha)),
					Beta:  float32(x.AtVec(StateIndexBeta)),
				},
			},
		}
		requestData := &analyzer.RequestSize{
			AvgInputTokens:  t.env.AvgInputToks,
			AvgOutputTokens: t.env.AvgOutputToks,
		}

		qa, err := analyzer.NewQueueAnalyzer(qConfig, requestData)
		if err != nil {
			logger.Log.Warn(err, "model tuner observation function: failed to create queue analyzer")
			return nil
		}

		lambda := t.env.Lambda / 60 // convert to req per sec
		metrics, err := qa.Analyze(lambda)
		if err != nil {
			logger.Log.Warn(err, "model tuner observation function: failed to analyze queueing model")
			return nil
		}

		ttft := float64(metrics.AvgWaitTime + metrics.AvgPrefillTime)
		itl := float64(metrics.AvgTokenTime)

		return mat.NewVecDense(2, []float64{ttft, itl})
	}
}

func (t *Tuner) extractTunedResults() (*TunedResults, error) {
	stateVec := mat.VecDenseCopyOf(t.X())
	if stateVec == nil {
		return nil, fmt.Errorf("tuner returned nil state vector")
	}
	innovation := mat.VecDenseCopyOf(t.Y())
	covariance := mat.DenseCopyOf(t.P())

	return &TunedResults{
		ServiceParms: &analyzer.ServiceParms{
			Decode: &analyzer.DecodeParms{
				Alpha: float32(stateVec.AtVec(StateIndexAlpha)),
				Beta:  float32(stateVec.AtVec(StateIndexBeta)),
			},
			Prefill: &analyzer.PrefillParms{
				Gamma: float32(stateVec.AtVec(StateIndexGamma)),
				Delta: float32(stateVec.AtVec(StateIndexDelta)),
			},
		},
		Innovation: innovation,
		Covariance: covariance,
	}, nil
}

func (t *Tuner) validateTunedResults(tunedResults *TunedResults) error {
	parms := tunedResults.ServiceParms

	// 1. check parms are positive
	if parms.Decode.Alpha <= 0 || parms.Decode.Beta <= 0 {
		return fmt.Errorf("decode parameters must be positive: alpha=%f, beta=%f", parms.Decode.Alpha, parms.Decode.Beta)
	}
	if parms.Prefill.Gamma <= 0 || parms.Prefill.Delta <= 0 {
		return fmt.Errorf("prefill parameters must be positive: gamma=%f, delta=%f", parms.Prefill.Gamma, parms.Prefill.Delta)
	}

	// 2. innovation check using Normalized Innovation Squared (NIS)
	innovation := mat.VecDenseCopyOf(t.Y()) // y vector
	innovationCov := mat.DenseCopyOf(t.S()) // S matrix

	// Calculate NIS = y^T * S^-1 * y
	S_inv := mat.NewDense(innovationCov.RawMatrix().Rows, innovationCov.RawMatrix().Cols, nil)
	if err := S_inv.Inverse(innovationCov); err != nil {
		return fmt.Errorf("singular innovation covariance matrix S encountered: %w", err)
	}

	// tmp = S^-1 * y
	tmp := mat.NewVecDense(S_inv.RawMatrix().Rows, nil)
	tmp.MulVec(S_inv, innovation)

	// NIS = y^T * tmp
	NIS := mat.Dot(innovation, tmp)
	tunedResults.NIS = NIS

	if NIS >= constants.DefaultMaxNIS {
		return fmt.Errorf("normalized innovation squared (NIS=%.2f) exceeds threshold (%.2f), rejecting update as outlier",
			NIS, constants.DefaultMaxNIS)
	}

	// 3. estimate covariance check?
	// TODO

	return nil
}
