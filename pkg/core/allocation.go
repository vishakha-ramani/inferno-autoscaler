package core

import (
	"fmt"
	"math"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/queue-analysis/pkg/queue"
	"github.ibm.com/tantawi/queue-analysis/pkg/utils"
)

// An allocation of a model on an accelerators
type Allocation struct {
	accelerator string  // name of accelerator
	numReplicas int     // number of server replicas
	batchSize   int     // max batch size
	cost        float32 // cost of this allocation
	value       float32 // value of this allocation
	servTime    float32 // expected average token service time
	waitTime    float32 // expected average request queueing time
	rho         float32 // expected busy server defined as (1 - probability of at least one request running)
}

// queueing model used in performance analysis
var queueModel *queue.MM1ModelStateDependent

// Create an allocation for a model on an accelerator; nil if not feasible
func CreateAllocation(m *Model, g *Accelerator, ml *config.LoadSpec) *Allocation {
	gName := g.name
	var ma *config.ModelAcceleratorSpec
	var exists bool
	if ma, exists = m.perfData[gName]; !exists {
		return nil
	}

	// calculate max batch size (N) based on average request length (K)
	K := ml.AvgLength
	N := ma.MaxBatchSize * ma.AtTokens / K
	if N < 1 {
		N = 1
	}
	maxQueue := ma.MaxBatchSize * config.MaxQueueToBatchRatio

	// distribution of token time assumed deterministic
	tokenTimeLimit := ml.SLO_ITL
	servTimeLimit := float32(K) * tokenTimeLimit
	// distribution of waiting time assumed exponential
	waitTimeLimit := ml.SLO_TTW / config.SLOMargin

	// calculate state-dependent service rate for queueuing model
	servRate := make([]float32, N)
	for n := 1; n <= N; n++ {
		servTime := ma.Alpha + ma.Beta*float32(n)
		servRate[n-1] = float32(n) / (servTime * float32(K))
	}

	// analyze queueuing model
	queueModel = queue.NewMM1ModelStateDependent(maxQueue, servRate)
	lambdaMin := servRate[0] * config.Delta
	lambdaMax := servRate[N-1] * (1 - config.Delta)

	// determine rate at which the average service time is below the service time limit
	lambdaStarService, ind, err := utils.BinarySearch(lambdaMin, lambdaMax, servTimeLimit, EvalServTime)
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}
	if ind < 0 {
		return nil // unattainable service time limit
	}

	// determine rate at which the average waiting time is below to the waiting time limit
	var lambdaStarWait float32
	lambdaStarWait, ind, err = utils.BinarySearch(lambdaMin, lambdaMax, waitTimeLimit, EvalWaitingTime)
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}
	if ind < 0 {
		return nil // unattainable waiting time limit
	}

	// arrival rate satisfying both service and waiting time SLOs
	lambdaStar := float32(math.Min(float64(lambdaStarService), float64(lambdaStarWait)))

	// calculate number of replicas
	totalLambda := ml.ArrivalRate / 60 / 1000
	numReplicas := int(math.Ceil(float64(totalLambda) / float64(lambdaStar)))
	cost := g.spec.Cost * float32(m.numUnits[gName]*numReplicas*g.spec.Multiplicity)

	// queueModel.Solve(lambdaStar, 1)
	// fmt.Printf("model=%s; accelerator=%s; lambdaMin=%v; lambdaMax=%v; servTimeLimit= %v; waitTimeLimit=%v; lambdaStarService=%v; lambdaStarWait=%v; lambdaStar=%v \n",
	// 	m.spec.Name, gName,
	// 	lambdaMin, lambdaMax, servTimeLimit, waitTimeLimit, lambdaStarService, lambdaStarWait, lambdaStar)
	// fmt.Println(queueModel)

	// calculate queue statistics
	lambda := totalLambda / float32(numReplicas)
	queueModel.Solve(lambda, 1)
	rho := queueModel.GetRho()
	servTime := queueModel.GetAvgServTime() / float32(K)
	wait := queueModel.GetAvgWaitTime()
	// fmt.Printf("numReplicas=%d; batchSize=%d; lambda=%v, tokenTime=%v; wait=%v; \n", numReplicas, N, lambda, servTime, wait)

	alloc := &Allocation{accelerator: gName, numReplicas: numReplicas, batchSize: N,
		cost: cost, servTime: servTime, waitTime: wait, rho: rho}
	alloc.SetValue(alloc.cost)
	return alloc
}

func EvalWaitingTime(x float32) (float32, error) {
	queueModel.Solve(x, 1)
	if !queueModel.IsValid() {
		return 0, fmt.Errorf("invalid model %v", queueModel)
	}
	return queueModel.GetAvgWaitTime(), nil
}

func EvalServTime(x float32) (float32, error) {
	queueModel.Solve(x, 1)
	if !queueModel.IsValid() {
		return 0, fmt.Errorf("invalid model %v", queueModel)
	}
	return queueModel.GetAvgServTime(), nil
}

// Create an allocation for a model on an accelerator; nil if not feasible
// (using G/G/m model approximation)
func CreateAllocationUsingGGm(m *Model, g *Accelerator, ml *config.LoadSpec) *Allocation {
	gName := g.name
	var d *config.ModelAcceleratorSpec
	var exists bool
	if d, exists = m.perfData[gName]; !exists {
		return nil
	}

	gamma := ((ml.ArrivalCOV * ml.ArrivalCOV) + (ml.ServiceCOV * ml.ServiceCOV)) / 2

	K := ml.AvgLength
	N := d.MaxBatchSize * d.AtTokens / K
	if N < 1 {
		N = 1
	}

	servTime := d.Alpha + d.Beta*float32(N)
	tokenTimeLimit := ml.SLO_ITL
	if servTime > tokenTimeLimit {
		return nil
	}

	waitTimeLimit := ml.SLO_TTW / config.SLOMargin

	xStar := float32(d.MaxBatchSize) * waitTimeLimit / (float32(K) * servTime * gamma)
	rhoStar := xStar / (1 + xStar)
	lambdaStar := rhoStar / (float32(K) * servTime)
	numReplicas := int(math.Ceil(float64(ml.ArrivalRate) / (float64(lambdaStar) * 60 * 1000)))
	cost := g.spec.Cost * float32(m.numUnits[gName]*numReplicas*g.spec.Multiplicity)

	rho := ml.ArrivalRate * float32(K) * servTime / (float32(numReplicas) * 60 * 1000)
	x := rho / (1 - rho)
	wait := (float32(K) * servTime) * gamma * x / float32(d.MaxBatchSize)

	alloc := &Allocation{accelerator: gName, numReplicas: numReplicas, batchSize: N,
		cost: cost, servTime: servTime, waitTime: wait, rho: rho}
	alloc.SetValue(alloc.cost)
	return alloc
}

func (a *Allocation) Scale(model *Model, accelerators map[string]*Accelerator, ml *config.LoadSpec) (alloc *Allocation, inc int) {
	g := accelerators[a.accelerator]
	if g == nil {
		return nil, 0
	}
	alloc = CreateAllocation(model, g, ml)
	inc = alloc.numReplicas - a.numReplicas
	return alloc, inc
}

func (a *Allocation) ReAllocate(model *Model, accelerators map[string]*Accelerator, ml *config.LoadSpec) (*Allocation, string) {
	minVal := float32(0)
	var minAlloc *Allocation
	for _, g := range accelerators {
		if alloc := CreateAllocation(model, g, ml); alloc != nil {
			if minVal == 0 || alloc.value < minVal {
				minVal = alloc.value
				minAlloc = alloc
			}
		}
	}
	if minAlloc == nil {
		return nil, ""
	}
	return minAlloc, minAlloc.accelerator
}

// Set the value for this allocation (may depend on cost, performance, ...)
func (a *Allocation) SetValue(value float32) {
	a.value = value
}

// Calculate penalty for transitioning from this allocation (a) to another allocation (b)
func (a *Allocation) TransitionPenalty(b *Allocation) float32 {
	if a.accelerator == b.accelerator {
		if a.numReplicas == b.numReplicas {
			return 0
		} else {
			return b.cost - a.cost
		}
	}
	return 0.1*(a.cost+b.cost) + (b.cost - a.cost)
}

func (a *Allocation) String() string {
	return fmt.Sprintf("{acc=%s; num=%d; maxBatch=%d; cost=%v, val=%v, servTime=%v, waitTime=%v, rho=%v}",
		a.accelerator, a.numReplicas, a.batchSize, a.cost, a.value, a.servTime, a.waitTime, a.rho)
}
