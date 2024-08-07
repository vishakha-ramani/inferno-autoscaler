package core

import (
	"fmt"
	"math"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/queue-analysis/pkg/queue"
	"github.ibm.com/tantawi/queue-analysis/pkg/utils"
)

type Allocation struct {
	Accelerator string
	NumReplicas int
	BatchSize   int
	Cost        float32
	ServTime    float32
	WaitTime    float32
	Rho         float32
}

var queueModel *queue.MM1ModelStateDependent

func (a *Allocation) String() string {
	return fmt.Sprintf("Allocation: name=%s; num=%d; batch=%d; cost=%v, servTime=%v, waitTime=%v, rho=%v",
		a.Accelerator, a.NumReplicas, a.BatchSize, a.Cost, a.ServTime, a.WaitTime, a.Rho)
}

// Create an allocation for a model on an accelerator; nil if not feasible
func CreateAllocation(m *Model, g *Accelerator, ml *config.ModelLoadSpec) *Allocation {
	gName := g.Name
	var d *config.ModelPerfData
	var exists bool
	if d, exists = m.perfData[gName]; !exists {
		return nil
	}

	K := ml.AvgLength
	N := d.MaxBatchSize * d.AtTokens / K
	if N < 1 {
		N = 1
	}
	max := d.MaxBatchSize * config.MaxQueueToBatchRatio

	tokenTimeLimit := ml.SLO_ITL
	servTimeLimit := float32(K) * tokenTimeLimit
	waitTimeLimit := ml.SLO_TTW / config.SLOMargin

	servRate := make([]float32, N)
	for n := 1; n <= N; n++ {
		servTime := d.Alpha + d.Beta*float32(n)
		servRate[n-1] = float32(n) / (servTime * float32(K))
	}

	queueModel = queue.NewMM1ModelStateDependent(max, servRate)
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

	lambdaStar := float32(math.Min(float64(lambdaStarService), float64(lambdaStarWait)))
	queueModel.Solve(lambdaStar, 1)

	// fmt.Printf("model=%s; accelerator=%s; lambdaMin=%v; lambdaMax=%v; servTimeLimit= %v; waitTimeLimit=%v; lambdaStarService=%v; lambdaStarWait=%v; lambdaStar=%v \n",
	// 	modelName, gName,
	// 	lambdaMin, lambdaMax, servTimeLimit, waitTimeLimit, lambdaStarService, lambdaStarWait, lambdaStar)
	// fmt.Println(queueModel)

	totalLambda := ml.ArrivalRate / 60 / 1000
	numReplicas := int(math.Ceil(float64(totalLambda) / float64(lambdaStar)))
	cost := g.Spec.Cost * float32(m.numUnits[gName]*numReplicas*g.Spec.Multiplicity)

	lambda := totalLambda / float32(numReplicas)
	queueModel.Solve(lambda, 1)
	rho := queueModel.GetRho()
	servTime := queueModel.GetAvgServTime() / float32(K)
	wait := queueModel.GetAvgWaitTime()

	// fmt.Printf("numReplicas=%d; batchSize=%d; lambda=%v, tokenTime=%v; wait=%v; \n", numReplicas, N, lambda, servTime, wait)

	return &Allocation{Accelerator: gName, NumReplicas: numReplicas, BatchSize: N,
		Cost: cost, ServTime: servTime, WaitTime: wait, Rho: rho}
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
func CreateAllocationUsingGGm(m *Model, g *Accelerator, ml *config.ModelLoadSpec) *Allocation {
	gName := g.Name
	var d *config.ModelPerfData
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
	cost := g.Spec.Cost * float32(m.numUnits[gName]*numReplicas*g.Spec.Multiplicity)

	rho := ml.ArrivalRate * float32(K) * servTime / (float32(numReplicas) * 60 * 1000)
	x := rho / (1 - rho)
	wait := (float32(K) * servTime) * gamma * x / float32(d.MaxBatchSize)

	return &Allocation{Accelerator: gName, NumReplicas: numReplicas, BatchSize: N,
		Cost: cost, ServTime: servTime, WaitTime: wait, Rho: rho}
}
