package tuner

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

// Tuner configuration data
type TunerConfigData struct {
	FilterData FilterData     // filter data
	ModelData  TunerModelData // model data
}

// Filter configuration data
type FilterData struct {
	GammaFactor float64 // gamma factor
	ErrorLevel  float64 // error level percentile
	TPercentile float64 // tail of student distribution
}

// Model configuration data
// TODO: add P
// TODO: change InitState name
type TunerModelData struct {
	InitState            []float64  // initial state of model parameters
	CovarianceMatrix     *mat.Dense // covariance matrix
	PercentChange        []float64  // percent change in state
	BoundedState         bool       // are the state values bounded
	MinState             []float64  // lower bound on state
	MaxState             []float64  // upper bound on state
	ExpectedObservations []float64  // expected values of observations
}

// Configurator for the model tuner
type Configurator struct {
	// dimensions
	nX int // number of state parameters
	nZ int // number of observation metrics

	// matrices
	X0 *mat.VecDense // initial values of state parameters
	P  *mat.Dense    // covariance matrix of estimation error
	Q  *mat.Dense    // covariance matrix of noise on state
	R  *mat.Dense    // covariance matrix of noise on observation

	// functions
	fFunc func(*mat.VecDense) *mat.VecDense // transition function for the state params

	// other
	percentChange []float64 // expected percent change in state params
	Xbounded      bool      // if state bounded
	Xmin          []float64 // min values of state params
	Xmax          []float64 // max values of state params
}

func NewConfigurator(configData *TunerConfigData) (c *Configurator, err error) {
	if !checkConfigData(configData) {
		return nil, fmt.Errorf("invalid config data")
	}

	md := configData.ModelData
	n := len(md.InitState)
	X0 := mat.NewVecDense(n, md.InitState)

	fd := configData.FilterData
	m := len(md.ExpectedObservations)
	obsCOV := make([]float64, m)
	factor := ((fd.ErrorLevel / fd.TPercentile) * (fd.ErrorLevel / fd.TPercentile)) / fd.GammaFactor
	for j := range m {
		obsCOV[j] = factor * md.ExpectedObservations[j] * md.ExpectedObservations[j]
	}
	R := mat.DenseCopyOf(mat.NewDiagDense(m, obsCOV))

	c = &Configurator{
		nX:            n,
		nZ:            m,
		X0:            X0,
		P:             nil,
		Q:             nil,
		R:             R,
		fFunc:         nil,
		percentChange: md.PercentChange,
		Xbounded:      md.BoundedState,
		Xmin:          md.MinState,
		Xmax:          md.MaxState,
	}

	// Initialize P: use provided covariance if available, otherwise compute from state
	if md.CovarianceMatrix != nil {
		c.P = mat.DenseCopyOf(md.CovarianceMatrix)
	} else {
		c.P, err = c.GetStateCov(X0)
		if err != nil {
			return nil, err
		}
	}

	if c.Q, err = c.GetStateCov(X0); err != nil {
		return nil, err
	}
	c.fFunc = stateTransitionFunc
	return c, nil
}

func (c *Configurator) GetStateCov(x *mat.VecDense) (*mat.Dense, error) {
	if x.Len() != c.nX {
		return nil, mat.ErrNormOrder
	}
	changeCov := make([]float64, c.nX)
	for i := 0; i < c.nX; i++ {
		changeCov[i] = math.Pow(c.percentChange[i]*x.AtVec(i), 2)
	}
	return mat.DenseCopyOf(mat.NewDiagDense(c.nX, changeCov)), nil
}

func (c *Configurator) NumStates() int {
	return c.nX
}

func (c *Configurator) NumObservations() int {
	return c.nZ
}

func checkConfigData(cd *TunerConfigData) bool {
	if cd == nil {
		return false
	}

	// Validate FilterData
	fd := cd.FilterData
	if fd.GammaFactor <= 0 {
		return false
	}
	if fd.ErrorLevel <= 0 {
		return false
	}
	if fd.TPercentile <= 0 {
		return false
	}

	// Validate ModelData
	md := cd.ModelData
	n := len(md.InitState)
	if n == 0 {
		return false
	}

	// Check PercentChange length and values
	if len(md.PercentChange) != n {
		return false
	}
	for _, pc := range md.PercentChange {
		if pc <= 0 || math.IsNaN(pc) || math.IsInf(pc, 0) {
			return false
		}
	}

	// Check InitState values are valid
	for _, val := range md.InitState {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return false
		}
	}

	// Check bounded state constraints
	if md.BoundedState {
		if len(md.MinState) != n || len(md.MaxState) != n {
			return false
		}
		// Validate MinState < MaxState for each element
		for i := range n {
			if md.MinState[i] >= md.MaxState[i] {
				return false
			}
			if math.IsNaN(md.MinState[i]) || math.IsInf(md.MinState[i], 0) {
				return false
			}
			if math.IsNaN(md.MaxState[i]) || math.IsInf(md.MaxState[i], 0) {
				return false
			}
		}
	}

	// Check ExpectedObservations
	if len(md.ExpectedObservations) == 0 {
		return false
	}
	for _, obs := range md.ExpectedObservations {
		if obs <= 0 || math.IsNaN(obs) || math.IsInf(obs, 0) {
			return false
		}
	}

	return true
}

func stateTransitionFunc(x *mat.VecDense) *mat.VecDense {
	return x
}
