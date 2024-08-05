package core

import (
	"fmt"
	"math"

	"github.ibm.com/tantawi/queue-analysis/pkg/queue"
	"github.ibm.com/tantawi/queue-analysis/pkg/utils"
)

type ServiceClass struct {
	Spec *ServiceClassSpec

	// for all models, for all accelerators
	AllAllocations map[string]map[string]*Allocation

	// gamma for models
	gamma map[string]float32

	// allocated solution for all models
	allocation map[string]*Allocation
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

type Allocation struct {
	Accelerator string
	NumReplicas int
	BatchSize   int
	Cost        float32
	ServTime    float32
	WaitTime    float32
	Rho         float32
}

func (a *Allocation) String() string {
	return fmt.Sprintf("Allocation: name=%s; num=%d; batch=%d; cost=%v, servTime=%v, waitTime=%v, rho=%v",
		a.Accelerator, a.NumReplicas, a.BatchSize, a.Cost, a.ServTime, a.WaitTime, a.Rho)
}

func NewServiceClassFromSpec(spec *ServiceClassSpec) *ServiceClass {
	return &ServiceClass{
		Spec:           spec,
		AllAllocations: make(map[string]map[string]*Allocation),
		gamma:          make(map[string]float32),
		allocation:     map[string]*Allocation{},
	}
}

// CalculateGGm basic parameters
func (c *ServiceClass) CalculateGGm(models map[string]*Model, accelerators map[string]*Accelerator) {
	for i, v := range c.Spec.ModelLoad {
		modelName := v.Name
		model := models[modelName]
		c.gamma[modelName] = ((v.ArrivalCOV * v.ArrivalCOV) + (v.ServiceCOV * v.ServiceCOV)) / 2
		c.AllAllocations[modelName] = make(map[string]*Allocation)

		for gName, g := range accelerators {
			maxBatchSize := model.Spec.MaxBatchSize * model.Spec.AtTokens / c.Spec.ModelLoad[i].AvgLength
			N := int(float32(maxBatchSize) * model.MaxBatchSizeMultiplier[gName])
			if N < 1 {
				N = 1
			}
			profiledServTime := model.Spec.Alpha + model.Spec.Beta*float32(N)
			servTime := profiledServTime * model.serviceTimeMultiplier[gName]

			if servTime > c.Spec.ModelLoad[i].SLO_ITL {
				continue
			}

			xStar := float32(model.Spec.MaxBatchSize) * c.Spec.ModelLoad[i].SLO_TTW / (float32(v.AvgLength) * servTime * c.gamma[modelName])
			rhoStar := xStar / (1 + xStar)
			lambdaStar := rhoStar / (float32(v.AvgLength) * servTime)
			numReplicas := int(math.Ceil(float64(v.ArrivalRate) / (float64(lambdaStar) * 60 * 1000)))
			cost := g.Spec.Cost * float32(model.numUnits[gName]*numReplicas*g.Spec.Multiplicity)

			rho := v.ArrivalRate * float32(v.AvgLength) * servTime / (float32(numReplicas) * 60 * 1000)
			x := rho / (1 - rho)
			wait := (float32(v.AvgLength) * servTime) * c.gamma[modelName] * x / float32(model.Spec.MaxBatchSize)

			c.AllAllocations[modelName][gName] = &Allocation{Accelerator: gName, NumReplicas: numReplicas, BatchSize: N,
				Cost: cost, ServTime: servTime, WaitTime: wait, Rho: rho}
		}
	}
}

var queueModel *queue.MM1ModelStateDependent

func (c *ServiceClass) Calculate(models map[string]*Model, accelerators map[string]*Accelerator) {
	for i, v := range c.Spec.ModelLoad {
		modelName := v.Name
		model := models[modelName]
		// c.gamma[modelName] = ((v.ArrivalCOV * v.ArrivalCOV) + (v.ServiceCOV * v.ServiceCOV)) / 2
		c.AllAllocations[modelName] = make(map[string]*Allocation)

		delta := float32(0.001)

		for gName, g := range accelerators {

			alpha := model.Spec.Alpha * model.serviceTimeMultiplier[gName]
			beta := model.Spec.Beta * model.serviceTimeMultiplier[gName]
			maxBatchSize := model.Spec.MaxBatchSize * model.Spec.AtTokens / c.Spec.ModelLoad[i].AvgLength
			N := int(float32(maxBatchSize) * model.MaxBatchSizeMultiplier[gName])
			if N < 1 {
				N = 1
			}

			K := v.AvgLength
			max := model.Spec.MaxBatchSize * 10

			tokenTimeLimit := c.Spec.ModelLoad[i].SLO_ITL
			servTimeLimit := float32(K) * tokenTimeLimit
			waitTimeLimit := c.Spec.ModelLoad[i].SLO_TTW

			servRate := make([]float32, N)
			for n := 1; n <= N; n++ {
				servRate[n-1] = float32(n) / ((alpha + beta*float32(n)) * float32(K))
			}

			queueModel = queue.NewMM1ModelStateDependent(max, servRate)
			lambdaMin := servRate[0] * delta
			lambdaMax := servRate[N-1] * (1 - delta)

			// determine rate at which the average service time is below the service time limit
			lambdaStarService, ind, err := utils.BinarySearch(lambdaMin, lambdaMax, servTimeLimit, EvalServTime)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			if ind < 0 {
				continue // unattainable service time limit
			}

			// determine rate at which the average waiting time is below to the waiting time limit
			var lambdaStarWait float32
			lambdaStarWait, ind, err = utils.BinarySearch(lambdaMin, lambdaMax, waitTimeLimit, EvalWaitingTime)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			if ind < 0 {
				continue // unattainable waiting time limit
			}

			lambdaStar := float32(math.Min(float64(lambdaStarService), float64(lambdaStarWait)))
			queueModel.Solve(lambdaStar, 1)

			// fmt.Printf("serviceClass=%s; model=%s; accelerator=%s; lambdaMin=%v; lambdaMax=%v; servTimeLimit= %v; waitTimeLimit=%v; lambdaStarService=%v; lambdaStarWait=%v; lambdaStar=%v \n",
			// 	c.Spec.Name, modelName, gName,
			// 	lambdaMin, lambdaMax, servTimeLimit, waitTimeLimit, lambdaStarService, lambdaStarWait, lambdaStar)
			// fmt.Println(queueModel)

			totalLambda := v.ArrivalRate / 60 / 1000
			numReplicas := int(math.Ceil(float64(totalLambda) / float64(lambdaStar)))
			cost := g.Spec.Cost * float32(model.numUnits[gName]*numReplicas*g.Spec.Multiplicity)

			lambda := totalLambda / float32(numReplicas)
			queueModel.Solve(lambda, 1)
			rho := queueModel.GetRho()
			servTime := queueModel.GetAvgServTime() / float32(K)
			wait := queueModel.GetAvgWaitTime()

			// fmt.Printf("numReplicas=%d; batchSize=%d; lambda=%v, tokenTime=%v; wait=%v; \n", numReplicas, N, lambda, servTime, wait)

			c.AllAllocations[v.Name][gName] = &Allocation{Accelerator: gName, NumReplicas: numReplicas, BatchSize: N,
				Cost: cost, ServTime: servTime, WaitTime: wait, Rho: rho}
		}
	}
}

func (c *ServiceClass) String() string {
	return fmt.Sprintf("ServiceClass: name=%s; load=%v; gamma=%v; allAllocations=%v; allocation=%v",
		c.Spec.Name, c.Spec.ModelLoad, c.gamma, c.AllAllocations, c.allocation)
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
