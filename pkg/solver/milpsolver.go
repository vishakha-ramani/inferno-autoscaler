package solver

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/core"
	lpsolveConfig "github.ibm.com/tantawi/lpsolve/pkg/config"
	lpsolve "github.ibm.com/tantawi/lpsolve/pkg/core"
	lpsolveUtils "github.ibm.com/tantawi/lpsolve/pkg/utils"
)

type MILPSolver struct {
	system        *core.System
	optimizerSpec *config.OptimizerSpec

	numServers             int         // number of servers (a pair of service class and model)
	numAccelerators        int         // number of accelerators
	instanceCost           []float64   // [numAccelerators]
	numInstancesPerReplica [][]int     // [sumServers][numAccelerators]
	ratePerReplica         [][]float64 // [sumServers][numAccelerators]
	arrivalRates           []float64   // [sumServers]

	numAcceleratorTypes    int     // number of accelerator types
	unitsAvail             []int   // [numAcceleratorTypes]
	acceleratorTypesMatrix [][]int // [numAcceleratorTypes][numAccelerators]

	numReplicas   [][]int // resulting number of replicas [numServers][numAccelerators]
	instancesUsed []int   // number of used accelerator units [numAccelerators]
	unitsUsed     []int   // [numAcceleratorTypes]

	accIndex        map[string]int            // acceleratorName -> index in accelerator arrays
	accLookup       []string                  // index -> acceleratorName
	serverIndex     map[string]map[string]int // serviceClassName -> modelName -> index in server arrays
	servClassLookup []string                  // index -> serviceClassName
	modelLookup     []string                  // index -> modelName
	accTypeIndex    map[string]int            // acceleratorTypeName -> index in acceleratorType arrays
	accTypeLookup   []string                  // index -> acceleratorTypeName
}

func NewMILPSolver(system *core.System, optimizerSpec *config.OptimizerSpec) *MILPSolver {
	return &MILPSolver{
		system:        system,
		optimizerSpec: optimizerSpec,
	}
}

func (v *MILPSolver) Solve() {
	v.preProcess()

	isLimited := !v.optimizerSpec.Unlimited
	isMulti := v.optimizerSpec.Heterogeneous
	useCplex := v.optimizerSpec.UseCplex
	v.optimize(isLimited, isMulti, useCplex)

	v.postProcess()
}

// prepare input date for MILP solver
func (v *MILPSolver) preProcess() {

	s := v.system

	// create map and lookup arrays for accelerators
	accMap := s.GetAccelerators()
	v.numAccelerators = len(accMap)
	v.accIndex = make(map[string]int)
	v.accLookup = make([]string, v.numAccelerators)

	// set cost values
	v.instanceCost = make([]float64, v.numAccelerators)
	index := 0
	for accName, acc := range accMap {
		v.accIndex[accName] = index
		v.accLookup[index] = accName
		v.instanceCost[index] = float64(acc.GetSpec().Cost)
		index++
	}

	// fmt.Println(v.accIndex)
	// fmt.Println(v.accLookup)
	// fmt.Println(lpsolveUtils.Pretty1D("unitCost", v.instanceCost))

	// create map and lookup arrays for accelerator types
	capMap := s.GetCapacity()
	v.numAcceleratorTypes = len(capMap)
	v.accTypeIndex = make(map[string]int)
	v.accTypeLookup = make([]string, v.numAcceleratorTypes)

	// set available accelerator types
	v.unitsAvail = make([]int, v.numAcceleratorTypes)
	v.acceleratorTypesMatrix = make([][]int, v.numAcceleratorTypes)
	index = 0
	for accTypeName, accTypeCount := range capMap {
		v.accTypeIndex[accTypeName] = index
		v.accTypeLookup[index] = accTypeName
		v.unitsAvail[index] = accTypeCount
		v.acceleratorTypesMatrix[index] = make([]int, v.numAccelerators)
		index++
	}

	// set matrix of accelerator types to accelerators
	for accName, acc := range accMap {
		accType := acc.GetType()
		if accIndex, exists := v.accIndex[accName]; exists {
			accTypeIndex := v.accTypeIndex[accType]
			v.acceleratorTypesMatrix[accTypeIndex][accIndex] = acc.GetSpec().Multiplicity
		}
	}

	// fmt.Println(v.accTypeIndex)
	// fmt.Println(v.accTypeLookup)
	// fmt.Println(lpsolveUtils.Pretty1D("unitsAvailByType", v.unitsAvail))
	// fmt.Println(lpsolveUtils.Pretty2D("acceleratorTypesMatrix", v.acceleratorTypesMatrix))

	// create map and lookup arrays for servers (service classes, models)
	index = 0
	v.serverIndex = make(map[string]map[string]int)
	scMap := s.GetServiceClasses()
	for scName, sc := range scMap {
		v.serverIndex[scName] = make(map[string]int)
		for mName, allocMap := range sc.GetAllAllocations() {
			if len(allocMap) > 0 {
				v.serverIndex[scName][mName] = index
				index++
			}
		}
	}
	v.numServers = index
	v.servClassLookup = make([]string, v.numServers)
	v.modelLookup = make([]string, v.numServers)
	for scName, mMap := range v.serverIndex {
		for mName, index := range mMap {
			v.servClassLookup[index] = scName
			v.modelLookup[index] = mName
		}
	}

	// set values for arrival rates and per replica arrivals and number of instances
	v.arrivalRates = make([]float64, v.numServers)
	v.numInstancesPerReplica = make([][]int, v.numServers)
	v.ratePerReplica = make([][]float64, v.numServers)
	for i := 0; i < v.numServers; i++ {
		v.numInstancesPerReplica[i] = make([]int, v.numAccelerators)
		v.ratePerReplica[i] = make([]float64, v.numAccelerators)
	}
	modelMap := s.GetModels()
	for scName, sc := range scMap {
		for mName, ml := range sc.GetModelLoads() {
			if i, exists := v.serverIndex[scName][mName]; exists {
				v.arrivalRates[i] = float64(ml.ArrivalRate / 60 / 1000)
				m := modelMap[mName]
				for accName, j := range v.accIndex {
					//acc := accMap[accName]
					v.numInstancesPerReplica[i][j] = m.GetNumInstances(accName)
					if alloc := sc.GetAllocationForPair(mName, accName); alloc != nil {
						v.ratePerReplica[i][j] = float64(alloc.GetMaxArrvRatePerReplica())
					}
				}
			}
		}
	}

	// fmt.Println(v.serverIndex)
	// fmt.Println(lpsolveUtils.Pretty1D("arrivalRates", v.arrivalRates))
	// fmt.Println(lpsolveUtils.Pretty2D("ratePerReplica", v.ratePerReplica))
	// fmt.Println(lpsolveUtils.Pretty2D("numInstancesPerReplica", v.numInstancesPerReplica))
}

// call MILP solver to optimize problem
func (v *MILPSolver) optimize(isLimited bool, isMulti bool, useCplex bool) {
	problemType := lpsolveConfig.SINGLE
	if isMulti {
		problemType = lpsolveConfig.MULTI
	}
	if p, err := v.createProblem(problemType, isLimited, useCplex); err != nil || p.Solve() != nil {
		fmt.Println(err)
	} else {
		v.printResults(problemType, p)
	}
}

func (v *MILPSolver) createProblem(problemType lpsolveConfig.ProblemType, isLimited bool, useCplex bool) (lpsolve.Problem, error) {
	// create a new problem instance
	var p lpsolve.Problem
	var err error
	switch problemType {
	case lpsolveConfig.SINGLE:
		if useCplex {
			p, err = lpsolve.CreateCplexProblem(v.numServers, v.numAccelerators, v.instanceCost, v.numInstancesPerReplica,
				v.ratePerReplica, v.arrivalRates)
		} else {
			p, err = lpsolve.CreateSingleAssignProblem(v.numServers, v.numAccelerators, v.instanceCost, v.numInstancesPerReplica,
				v.ratePerReplica, v.arrivalRates)
		}
	case lpsolveConfig.MULTI:
		if useCplex {
			p, err = lpsolve.CreateCplexProblem(v.numServers, v.numAccelerators, v.instanceCost, v.numInstancesPerReplica,
				v.ratePerReplica, v.arrivalRates)
		} else {
			p, err = lpsolve.CreateMultiAssignProblem(v.numServers, v.numAccelerators, v.instanceCost, v.numInstancesPerReplica,
				v.ratePerReplica, v.arrivalRates)
		}
	default:
		return nil, fmt.Errorf("unknown problem type: %s", problemType)
	}
	if err != nil {
		return nil, err
	}

	// set accelerator count limited option
	if isLimited {
		if err = p.SetLimited(v.numAcceleratorTypes, v.unitsAvail, v.acceleratorTypesMatrix); err != nil {
			return nil, err
		}
		if useCplex {
			switch problemType {
			case lpsolveConfig.SINGLE:
				SetFileNames(p, "single-limited")
			case lpsolveConfig.MULTI:
				SetFileNames(p, "multi-limited")
			}
		}
	} else {
		p.UnSetLimited()
		if useCplex {
			switch problemType {
			case lpsolveConfig.SINGLE:
				SetFileNames(p, "single-unlimited")
			case lpsolveConfig.MULTI:
				SetFileNames(p, "multi-unlimited")
			}
		}
	}
	return p, nil
}

func SetFileNames(p lpsolve.Problem, name string) {
	pc := p.(*lpsolve.CplexProblem)
	pc.SetModelFileName(name + ".mod")
	pc.SetDataFileName(name + ".dat")
	pc.SetOutputFileName(name + ".txt")
}

// print solution details
func (v *MILPSolver) printResults(problemType lpsolveConfig.ProblemType, p lpsolve.Problem) {
	fmt.Printf("Problem type: %v\n", problemType)
	fmt.Printf("Solution type: %v\n", p.GetSolutionType())
	fmt.Printf("Solution time: %d msec\n", p.GetSolutionTimeMsec())
	fmt.Printf("Objective value: %v\n", p.GetObjectiveValue())

	fmt.Println()
	fmt.Printf("Accelerators=%v \n", v.accLookup)
	fmt.Printf("ServiceClasses=%v \n", v.servClassLookup)
	fmt.Printf("Models=%v \n", v.modelLookup)
	fmt.Println()

	v.numReplicas = p.GetNumReplicas()
	fmt.Println(lpsolveUtils.Pretty2D("numReplicas", v.numReplicas))

	v.instancesUsed = p.GetInstancesUsed()
	fmt.Println(lpsolveUtils.Pretty1D("instancesUsed", v.instancesUsed))

	if p.IsLimited() {
		fmt.Printf("AcceleratorTypes=%v \n", v.accTypeLookup)
		fmt.Println(lpsolveUtils.Pretty1D("unitsAvail", v.unitsAvail))
		v.unitsUsed = p.GetUnitsUsed()
		fmt.Println(lpsolveUtils.Pretty1D("unitsUsed", v.unitsUsed))
	}
	fmt.Println()
}

// process output date from MILP solver
func (v *MILPSolver) postProcess() {
	s := v.system

	for i := 0; i < v.numServers; i++ {
		scName := v.servClassLookup[i]
		mName := v.modelLookup[i]
		for j := 0; j < v.numAccelerators; j++ {
			n := v.numReplicas[i][j]
			if n == 0 {
				continue
			}
			accName := v.accLookup[j]
			sc := s.GetServiceClass(scName)
			// TODO: Fix this
			if alloc := sc.GetAllocationForPair(mName, accName); alloc != nil {
				sc.SetAllocation(mName, alloc)
			}
		}
	}
}
