package core

type AcceleratorData struct {
	Spec  map[string]AcceleratorSpec `json:"spec"`
	Count []AcceleratorCount         `json:"count"`
}

type AcceleratorCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type ModelData struct {
	Spec []ModelSpec `json:"spec"`
}

type ServiceClassData struct {
	Spec []ServiceClassSpec `json:"spec"`
}
