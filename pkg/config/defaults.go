package config

import "math"

/**
 * Environment variables
 */

// REST server host env name
const RestHostEnvName = "INFERNO_HOST"
const DefaultRestHost = "localhost"

// REST server port env name
const RestPortEnvName = "INFERNO_PORT"
const DefaultRestPort = "8080"

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
