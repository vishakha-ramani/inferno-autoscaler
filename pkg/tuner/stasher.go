package tuner

import (
	kalman "github.com/llm-inferno/kalman-filter/pkg/core"
	"gonum.org/v1/gonum/mat"
)

type Stasher struct {
	Filter *kalman.ExtendedKalmanFilter
	X      *mat.VecDense // State vector (Xdim)
	P      *mat.Dense    // Estimate uncertainty covariance (Xdim x Xdim)
}

func NewStasher(filter *kalman.ExtendedKalmanFilter) *Stasher {
	return &Stasher{
		Filter: filter,
	}
}

func (s *Stasher) Stash() {
	// copy X and P from filter to the stasher
	s.X = mat.VecDenseCopyOf(s.Filter.X)
	s.P = mat.DenseCopyOf(s.Filter.P)
}

func (s *Stasher) UnStash() {
	// copy X and P from stasher to filter
	s.Filter.X = mat.VecDenseCopyOf(s.X)
	s.Filter.P = mat.DenseCopyOf(s.P)
}
