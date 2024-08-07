package config

import "math"

// Tolerated percentile for SLOs
var SLOPercentile = 0.95

// Multiplier of average of exponential distribution to attain percentile
var SLOMargin = -float32(math.Log(1 - SLOPercentile))

// small disturbance around a value
var Delta = float32(0.001)

// maximum number of requests in system as multiples of maximum batch size
var MaxQueueToBatchRatio = 10
