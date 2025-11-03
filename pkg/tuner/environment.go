package tuner

import (
	"bytes"
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

// Representation of the environment in which the system operates
type Environment struct {
	Lambda        float32 // request arrival rate (per minute)
	AvgInputToks  int     // average number of prompt (input) tokens per request
	AvgOutputToks int     // average number of output tokens per request
	MaxBatchSize  int     // maximum batch size
	AvgTTFT       float32 // average request queueing time (msec)
	AvgITL        float32 // average inter token latency (msec)
}

func (e *Environment) Valid() bool {
	return e.Lambda > 0 &&
		!math.IsInf(float64(e.Lambda), 0) &&
		!math.IsNaN(float64(e.Lambda)) &&
		e.AvgInputToks > 0 &&
		e.AvgOutputToks > 0 &&
		e.MaxBatchSize > 0 &&
		e.AvgTTFT > 0 &&
		e.AvgITL > 0
}

func (e *Environment) GetObservations() *mat.VecDense {
	return mat.NewVecDense(2, []float64{float64(e.AvgTTFT), float64(e.AvgITL)})
}

func (e *Environment) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Environment: ")
	fmt.Fprintf(&b, "rpm=%5.2f; avgInputToks=%d; avgOutputToks=%d; maxBatch=%d; avgTTFT=%10.6f; avgITL=%10.6f",
		e.Lambda, e.AvgInputToks, e.AvgOutputToks, e.MaxBatchSize, e.AvgTTFT, e.AvgITL)
	return b.String()
}
