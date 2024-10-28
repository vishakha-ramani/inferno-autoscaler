package client

import "os"

const (
	// environment variables names
	DataPathEnvName      = "INFERNO_DATA_PATH"
	ControlPeriodEnvName = "INFERNO_CONTROL_PERIOD"

	// path to static data json files (ends with /)
	DefaultDataPath = "./"

	// static data file names
	AcceleratorFileName  = "accelerator-data.json"
	ModelFileName        = "model-data.json"
	ServiceClassFileName = "serviceclass-data.json"
	OptimizerFileName    = "optimizer-data.json"

	ServerFileName = "server-data.json"

	// API settings
	OptimizeVerb = "optimizeOne"
	ServersVerb  = "getServers"

	// others
	DefaultControlPeriodSeconds int = 60
)

var (
	ControlPeriodSeconds = os.Getenv(ControlPeriodEnvName)

	OptimizerURL string
	DataPath     string
)
