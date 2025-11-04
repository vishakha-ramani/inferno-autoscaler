package constants

// Default tuner parameters
const (
	DefaultGammaFactor    = 1.0
	DefaultErrorLevel     = 0.05
	DefaultTPercentile    = 1.96
	DefaultPercentChange  = 0.05
	DefaultMinStateFactor = 0.1
	DefaultMaxStateFactor = 10
	/*
		Under nominal conditions, the NIS (Normalized Innovations Squared) of a Kalman Filter is expected to follow
		a Chi-Squared Distribution with degrees of freedom equal to the dimension of the measurement vector (n = 2 for [ttft, itl]).
		Here, we enforce that a tuner update is accepted for 95% confidence interval of NIS.
		The upper bound of the interval in our case is 7.378.
	*/
	DefaultMaxNIS = 7.378

	// Transient delay to allow scaled up servers to start serving requests before tuning is applied
	TransientDelaySeconds = 120
)
