package config

import "math"

/**
 * Parameters
 */

// Tolerated percentile for SLOs
var SLOPercentile = 0.95

// Multiplier of average of exponential distribution to attain percentile
var SLOMargin = -float32(math.Log(1 - SLOPercentile))

// small disturbance around a value
var Delta = float32(0.001)

// maximum number of requests in queueing system as multiples of maximum batch size
var MaxQueueToBatchRatio = 10

// accelerator transition penalty factor
var AccelPenaltyFactor = float32(0.1)

// default priority of a service class
const DefaultServiceClassPriority int = 0

// weight factor for class priority used in greedy limited solver
var PriorityWeightFactor float32 = 1.0
