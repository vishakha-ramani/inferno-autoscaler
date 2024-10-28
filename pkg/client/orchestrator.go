package client

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// Implementor of optimizer decisions
type Orchestrator struct {
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{}
}

type Alloc struct {
	Accelerator  string  `json:"accelerator"`
	NumReplicas  int     `json:"numReplicas"`
	MaxBatchSize int     `json:"maxBatchSize"`
	RPM          float32 `json:"RPM"`
	Tokens       int     `json:"tokens"`
}

type Transition struct {
	Current *Alloc `json:"current"`
	Desired *Alloc `json:"desired"`
}

// realize desired allocation for all servers
//   - (currently, prints state change for testing only)
func (o *Orchestrator) Do(serverData *config.ServerData) {
	changeMap := make(map[string]*Transition)
	for i := range serverData.Spec {

		serverName := serverData.Spec[i].Name
		currentAlloc := serverData.Spec[i].CurrentAlloc
		desiredAlloc := serverData.Spec[i].DesiredAlloc
		// fmt.Printf("server=%s,\t currentAlloc=%v,\t desiredAlloc=%v \n", serverName, currentAlloc, desiredAlloc)

		current := &Alloc{
			Accelerator:  currentAlloc.Accelerator,
			NumReplicas:  currentAlloc.NumReplicas,
			MaxBatchSize: currentAlloc.MaxBatch,
			RPM:          currentAlloc.Load.ArrivalRate,
			Tokens:       currentAlloc.Load.AvgLength,
		}
		desired := &Alloc{
			Accelerator:  desiredAlloc.Accelerator,
			NumReplicas:  desiredAlloc.NumReplicas,
			MaxBatchSize: desiredAlloc.MaxBatch,
			RPM:          desiredAlloc.Load.ArrivalRate,
			Tokens:       desiredAlloc.Load.AvgLength,
		}
		transition := &Transition{
			Current: current,
			Desired: desired,
		}
		changeMap[serverName] = transition

		// make current allocation the desired one
		serverData.Spec[i].CurrentAlloc = serverData.Spec[i].DesiredAlloc
	}

	// print sorted change
	keys := make([]string, 0, len(changeMap))
	for k := range changeMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if transBytes, err := json.Marshal(*changeMap[k]); err == nil {
			fmt.Printf("%s: %v\n", k, string(transBytes))
		}
	}
	fmt.Println()
}
