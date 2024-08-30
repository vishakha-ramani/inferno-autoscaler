package core

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// System comprising all accelerators, models, and service classes
type System struct {
	accelerators   map[string]*Accelerator
	models         map[string]*Model
	serviceClasses map[string]*ServiceClass

	optimizer *Optimizer

	capacity         map[string]int               // available count of accelerator types
	allocationByType map[string]*AllocationByType // number of allocated accelerator types
}

// Data about allocated accelerator types
type AllocationByType struct {
	name  string  // name of accelerator type
	count int     // total number of this type
	limit int     // maximum number of this type
	cost  float32 // total cost of this type
}

// Create a new system
func NewSystem() *System {
	return &System{
		accelerators:   make(map[string]*Accelerator),
		models:         make(map[string]*Model),
		serviceClasses: make(map[string]*ServiceClass),

		capacity:         make(map[string]int),
		allocationByType: map[string]*AllocationByType{},
	}
}

// Set data about accelerators
func (s *System) SetAcceleratorsFromSpec(byteValue []byte) error {
	var d config.AcceleratorData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for k, v := range d.Spec {
		s.accelerators[k] = NewAcceleratorFromSpec(k, &v)
	}
	for _, v := range d.Count {
		if cap, exists := s.capacity[v.Type]; exists {
			s.capacity[v.Type] = cap + v.Count
		} else {
			s.capacity[v.Type] = v.Count
		}
	}
	return nil
}

// Set data about models
func (s *System) SetModelsFromSpec(byteValue []byte) error {
	var d config.ModelData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.models[v.Name] = NewModelFromSpec(&v)
	}
	return nil
}

// Set data about service classes
func (s *System) SetServiceClassesFromSpec(byteValue []byte) error {
	var d config.ServiceClassData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.serviceClasses[v.Name] = NewServiceClassFromSpec(&v)
	}
	return nil
}

// Create optimizer from spec
func (s *System) SetOptimizerFromSpec(byteValue []byte) error {
	var d config.OptimizerData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	s.optimizer = NewOptimizerFromSpec(&d.Spec)
	return nil
}

// Get accelerator object for a given accelerator name; nil if doesn't exist
func (s *System) GetAccelerator(name string) *Accelerator {
	return s.accelerators[name]
}

// Get all accelerators
func (s *System) GetAccelerators() map[string]*Accelerator {
	return s.accelerators
}

// Get model object for a given model name; nil if doesn't exist
func (s *System) GetModel(name string) *Model {
	return s.models[name]
}

// Get all models
func (s *System) GetModels() map[string]*Model {
	return s.models
}

// Get service class object for a given service class name; nil if doesn't exist
func (s *System) GetServiceClass(name string) *ServiceClass {
	return s.serviceClasses[name]
}

// Get all service classes
func (s *System) GetServiceClasses() map[string]*ServiceClass {
	return s.serviceClasses
}

// Calculate basic parameters
func (s *System) Calculate() {
	for _, g := range s.accelerators {
		g.Calculate()
	}
	for _, m := range s.models {
		m.Calculate(s.accelerators)
	}
	for _, c := range s.serviceClasses {
		c.Calculate(s.models, s.accelerators)
	}
}

func (s *System) Optimize() {
	s.optimizer.Optimize(s)
}

// Accumulate allocation data by accelerator type
func (s *System) AllocateByType() {
	s.allocationByType = map[string]*AllocationByType{}
	for _, c := range s.serviceClasses {
		for modelName, srvModelAlloc := range c.GetAllocations() {
			if srvModelAlloc == nil {
				continue
			}
			accName := srvModelAlloc.accelerator
			acc := s.accelerators[accName]
			model := s.GetModel(modelName)
			if acc == nil || model == nil {
				continue
			}
			nameType := acc.GetType()
			var alloc *AllocationByType
			var exists bool
			if alloc, exists = s.allocationByType[nameType]; !exists {
				alloc = &AllocationByType{
					name:  nameType,
					count: 0,
					limit: s.capacity[nameType],
					cost:  0,
				}
			}
			alloc.count += srvModelAlloc.numReplicas * model.numInstances[accName] * acc.spec.Multiplicity
			alloc.cost += srvModelAlloc.cost
			s.allocationByType[nameType] = alloc
		}
	}
}

// generate json allocation solution for all servers in the system
func (s *System) GetSolution() ([]byte, error) {
	allocationSolution := config.AllocationSolution{
		Spec: make(map[string]config.AllocationData),
	}
	for srvName, srv := range s.serviceClasses {
		for modelName, srvModelAlloc := range srv.GetAllocations() {
			if srvModelAlloc == nil {
				continue
			}
			allocData := config.AllocationData{
				ServiceClass: srvName,
				Model:        modelName,
				Accelerator:  srvModelAlloc.accelerator,
				NumReplicas:  srvModelAlloc.numReplicas,
				MaxBatch:     srvModelAlloc.batchSize,
				Cost:         srvModelAlloc.cost,
				ITLAverage:   srvModelAlloc.servTime,
				WaitAverage:  srvModelAlloc.waitTime,
			}
			allocationSolution.Spec[srvName+"/"+modelName] = allocData
		}
	}
	// generate json
	if byteValue, err := json.Marshal(allocationSolution); err != nil {
		return nil, err
	} else {
		return byteValue, nil
	}
}

func (a *AllocationByType) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "name=%s, count=%d, limit=%d, cost=%v", a.name, a.count, a.limit, a.cost)
	return b.String()
}

func (s *System) String() string {
	var b bytes.Buffer
	// b.WriteString("Accelerators: \n")
	// for _, g := range s.GetAccelerators() {
	// 	fmt.Fprintln(&b, g)
	// }
	// fmt.Fprintf(&b, "capacity=%v \n", s.capacity)
	// b.WriteString("Models: \n")
	// for _, m := range s.GetModels() {
	// 	fmt.Fprintln(&b, m)
	// }
	// b.WriteString("ServiceClasses: \n")
	// for _, c := range s.GetServiceClasses() {
	// 	fmt.Fprintln(&b, c)
	// }
	b.WriteString("Solution: \n")
	totalCost := float32(0)
	for srvClassName, c := range s.GetServiceClasses() {
		for modelName, modelLoadData := range c.GetModelLoads() {
			alloc := c.GetAllocation(modelName)
			if alloc == nil {
				fmt.Fprintf(&b, "c=%s; m=%s; no feasible allocation! \n", srvClassName, modelName)
				continue
			}
			totalCost += alloc.cost
			rate := modelLoadData.ArrivalRate
			tokens := modelLoadData.AvgLength
			fmt.Fprintf(&b, "c=%s; m=%s; rate=%v; tk=%d; sol=%d, alloc=%v; ", srvClassName, modelName, rate, tokens, len(c.allAllocations[modelName]), alloc)
			fmt.Fprintf(&b, "slo-itl=%v, slo-ttw=%v \n", modelLoadData.SLO_ITL, modelLoadData.SLO_TTW)
		}
	}
	b.WriteString("AllocationByType: \n")
	for _, a := range s.allocationByType {
		fmt.Fprintf(&b, "%v \n", a)
	}
	fmt.Fprintf(&b, "totalCost=%v \n", totalCost)
	if s.optimizer != nil {
		b.WriteString(s.optimizer.String())
	}
	return b.String()
}
