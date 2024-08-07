package config

type OptimizerData struct {
	Spec OptimizerSpec `json:"spec"`
}

type OptimizerSpec struct {
	Unlimited bool `json:"unlimited"`
}

type AcceleratorData struct {
	Spec  map[string]AcceleratorSpec `json:"spec"`
	Count []AcceleratorCount         `json:"count"`
}

type AcceleratorSpec struct {
	Type         string    `json:"type"`
	Multiplicity int       `json:"multiplicity"`
	MemSize      int       `json:"memSize"` // GB
	MemBW        int       `json:"memBW"`   // GB/sec
	Power        PowerSpec `json:"power"`
	Cost         float32   `json:"cost"` // cents/hr
}

type PowerSpec struct {
	Idle     int     `json:"idle"`
	Full     int     `json:"full"`
	MidPower int     `json:"midPower"`
	MidUtil  float32 `json:"midUtil"`
}

type AcceleratorCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type ModelData struct {
	Spec []ModelSpec `json:"spec"`
}

type ModelSpec struct {
	Name    string          `json:"name"`
	MemSize int             `json:"memSize"` // GB
	AccSpec []ModelPerfData `json:"perfData"`
}

type ModelPerfData struct {
	Name         string  `json:"name"`
	Alpha        float32 `json:"alpha"`
	Beta         float32 `json:"beta"`
	MaxBatchSize int     `json:"maxBatchSize"`
	AtTokens     int     `json:"atTokens"`
}

type ServiceClassData struct {
	Spec []ServiceClassSpec `json:"spec"`
}

type ServiceClassSpec struct {
	Name      string          `json:"name"`
	ModelLoad []ModelLoadSpec `json:"load"`
}

type ModelLoadSpec struct {
	Name        string  `json:"name"`
	SLO_ITL     float32 `json:"slo-itl"`     // msec
	SLO_TTW     float32 `json:"slo-ttw"`     // msec
	ArrivalRate float32 `json:"arrivalRate"` // req/min
	AvgLength   int     `json:"avgLength"`   // number of tokens
	ArrivalCOV  float32 `json:"arrivalCOV"`
	ServiceCOV  float32 `json:"serviceCOV"`
}
