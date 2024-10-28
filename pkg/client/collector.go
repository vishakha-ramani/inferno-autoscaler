package client

import (
	"math"
	"math/rand/v2"
	"os"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/utils"
)

// Collector of current server data
type Collector struct {
}

func NewCollector() *Collector {
	return &Collector{}
}

// initialize - after initializing Controller
func (c *Collector) Init() (*config.ServerData, error) {
	// read server data
	fn_srv := DataPath + ServerFileName
	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		return nil, err_srv
	}
	if d, err := utils.FromDataToSpec(bytes_srv, config.ServerData{}); err == nil {
		return d, nil
	} else {
		return nil, err
	}
}

// update dynamic data for all servers
func (c *Collector) Update(serverData *config.ServerData) error {
	for i := range serverData.Spec {
		perturbLoad(&serverData.Spec[i].CurrentAlloc.Load)
	}
	return nil
}

// randomly modify dynamic server data (testing only)
func perturbLoad(load *config.ServerLoadSpec) {
	// generate random values in [alpha, 2 - alpha), where 0 < alpha < 1
	alpha := float32(0.7)

	factorA := 2 * (rand.Float32() - 0.5) * (1 - alpha)
	newArv := load.ArrivalRate * (1 + factorA)
	if newArv <= 0 {
		newArv = 1
	}
	load.ArrivalRate = newArv

	factorB := 2 * (rand.Float32() - 0.5) * (1 - alpha)
	newLength := int(math.Ceil(float64(float32(load.AvgLength) * (1 + factorB))))
	if newLength <= 0 {
		newLength = 1
	}
	load.AvgLength = newLength
}
