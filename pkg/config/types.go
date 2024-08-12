package config

// Data related to Optimizer
type OptimizerData struct {
	Spec OptimizerSpec `json:"spec"`
}

// Specifications for optimizer data
type OptimizerSpec struct {
	Unlimited bool `json:"unlimited"` // unlimited number of accelerator types (for capacity planning and/or cloud)
}

// Data related to an Accelerator
type AcceleratorData struct {
	Spec  map[string]AcceleratorSpec `json:"spec"`  // map of accelerator names (e.g. A100, 2xA100) to specs
	Count []AcceleratorCount         `json:"count"` // count of accelerator types
}

// Specifications for accelerator data
type AcceleratorSpec struct {
	Type         string    `json:"type"`         // name of accelerator type (e.g. A100)
	Multiplicity int       `json:"multiplicity"` // number of cards of type for this accelerator
	MemSize      int       `json:"memSize"`      // GB
	MemBW        int       `json:"memBW"`        // GB/sec
	Power        PowerSpec `json:"power"`        // power consumption specs
	Cost         float32   `json:"cost"`         // cents/hr
}

// Specifications for Accelerator power consumption data (Watts)
type PowerSpec struct {
	Idle     int     `json:"idle"`     // idle power
	Full     int     `json:"full"`     // full utilization power
	MidPower int     `json:"midPower"` // power at inflection point
	MidUtil  float32 `json:"midUtil"`  // utilization at inflection point
}

// Count of accelerator types in the system
type AcceleratorCount struct {
	Type  string `json:"type"`  // name of accelerator type
	Count int    `json:"count"` // number of available units
}

// Data related to a Model
type ModelData struct {
	Spec []ModelSpec `json:"spec"`
}

// Specifications for model data
type ModelSpec struct {
	Name     string                     `json:"name"`     // name of model
	MemSize  int                        `json:"memSize"`  // GB
	PerfData []ModelAcceleratorPerfData `json:"perfData"` // performance data for model on accelerators
}

// Specifications for a combination of a model and accelerator data
type ModelAcceleratorPerfData struct {
	Name         string  `json:"name"`         // accelerator name
	Alpha        float32 `json:"alpha"`        // alpha parameter of ITL
	Beta         float32 `json:"beta"`         // beta parameter of ITL
	MaxBatchSize int     `json:"maxBatchSize"` // max batch size based on average number of tokens per request
	AtTokens     int     `json:"atTokens"`     // average number of tokens per request assumed in max batch size calculation
}

// Data related to a Service Class
type ServiceClassData struct {
	Spec []ServiceClassSpec `json:"spec"`
}

// Specifications for service class data
type ServiceClassSpec struct {
	Name string     `json:"name"` // service class name
	Load []LoadData `json:"load"`
}

// Specifications of load data for a combination of a service class and a model
type LoadData struct {
	Name        string  `json:"name"`        // model name
	SLO_ITL     float32 `json:"slo-itl"`     // msec
	SLO_TTW     float32 `json:"slo-ttw"`     // msec
	ArrivalRate float32 `json:"arrivalRate"` // req/min
	AvgLength   int     `json:"avgLength"`   // number of tokens
	ArrivalCOV  float32 `json:"arrivalCOV"`  // coefficient of variation of inter-request arrival time
	ServiceCOV  float32 `json:"serviceCOV"`  // coefficient of variation of request service time
}
