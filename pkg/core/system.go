package core

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

var (
	// the system object
	TheSystem *System
)

func GetAccelerator(name string) *Accelerator {
	return TheSystem.GetAccelerator(name)
}

func GetModel(name string) *Model {
	return TheSystem.GetModel(name)
}

func GetServiceClass(name string) *ServiceClass {
	return TheSystem.GetServiceClass(name)
}

func GetServer(name string) *Server {
	return TheSystem.GetServer(name)
}

func GetAccelerators() map[string]*Accelerator {
	return TheSystem.accelerators
}

func GetModels() map[string]*Model {
	return TheSystem.models
}

func GetServers() map[string]*Server {
	return TheSystem.servers
}

func GetCapacity() map[string]int {
	return TheSystem.capacity
}

// System comprising all accelerators, models, service classes, and servers
type System struct {
	accelerators   map[string]*Accelerator
	models         map[string]*Model
	serviceClasses map[string]*ServiceClass
	servers        map[string]*Server

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
		servers:        make(map[string]*Server),

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
	for _, v := range d.Spec {
		s.accelerators[v.Name] = NewAcceleratorFromSpec(v.Name, &v)
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
	for _, pd := range d.PerfData {
		if m := s.models[pd.Name]; m != nil {
			m.perfData[pd.Acc] = &pd
		}
	}
	return nil
}

// Set data about service classes
func (s *System) SetServiceClassesFromSpec(byteValue []byte) error {
	var d config.ServiceClassData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, t := range d.Spec {
		name := t.Name
		if _, exists := s.serviceClasses[name]; !exists {
			s.serviceClasses[name] = NewServiceClass(name)
		}
		svc := s.serviceClasses[name]
		svc.SetTargetFromSpec(&t)
	}
	return nil
}

// Set data about servers
func (s *System) SetServersFromSpec(byteValue []byte) error {
	var d config.ServerData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.servers[v.Name] = NewServerFromSpec(&v)
	}
	return nil
}

// Get all accelerators
func (s *System) GetAccelerators() map[string]*Accelerator {
	return s.accelerators
}

// Get all models
func (s *System) GetModels() map[string]*Model {
	return s.models
}

// Get all service classes
func (s *System) GetServiceClasses() map[string]*ServiceClass {
	return s.serviceClasses
}

// Get all servers
func (s *System) GetServers() map[string]*Server {
	return s.servers
}

// Get accelerator object for a given accelerator name; nil if doesn't exist
func (s *System) GetAccelerator(name string) *Accelerator {
	return s.accelerators[name]
}

// Get model object for a given model name; nil if doesn't exist
func (s *System) GetModel(name string) *Model {
	return s.models[name]
}

// Get service class object for a given service class name; nil if doesn't exist
func (s *System) GetServiceClass(name string) *ServiceClass {
	return s.serviceClasses[name]
}

// Get server object for a given server name; nil if doesn't exist
func (s *System) GetServer(name string) *Server {
	return s.servers[name]
}

// Get capacities of accelerator types
func (s *System) GetCapacity() map[string]int {
	return s.capacity
}

// Calculate basic parameters
func (s *System) Calculate() {
	for _, g := range s.accelerators {
		g.Calculate()
	}
	for _, m := range s.models {
		m.Calculate(s.accelerators)
	}
	for _, v := range s.servers {
		v.Calculate(s.accelerators)
	}
}

// Accumulate allocation data by accelerator type
func (s *System) AllocateByType() {
	s.allocationByType = map[string]*AllocationByType{}
	for _, server := range s.GetServers() {
		modelName := server.GetModelName()
		serverAlloc := server.GetAllocation()
		if serverAlloc == nil {
			continue
		}
		accName := serverAlloc.accelerator
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
		alloc.count += serverAlloc.numReplicas * model.numInstances[accName] * acc.spec.Multiplicity
		alloc.cost += serverAlloc.cost
		s.allocationByType[nameType] = alloc
	}
}

// generate json allocation solution for all servers in the system
func (s *System) GetSolution() ([]byte, error) {
	allocationSolution := config.AllocationSolution{
		Spec: make(map[string]config.AllocationData),
	}
	for serverName, server := range s.servers {
		serverAlloc := server.GetAllocation()
		if serverAlloc == nil {
			continue
		}
		allocData := config.AllocationData{
			ServiceClass: server.GetServiceClassName(),
			Model:        server.GetModelName(),
			Accelerator:  serverAlloc.accelerator,
			NumReplicas:  serverAlloc.numReplicas,
			MaxBatch:     serverAlloc.batchSize,
			Cost:         serverAlloc.cost,
			ITLAverage:   serverAlloc.servTime,
			WaitAverage:  serverAlloc.waitTime,
		}
		allocationSolution.Spec[serverName] = allocData
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
	// b.WriteString("Servers: \n")
	// for _, s := range s.GetServers() {
	// 	fmt.Fprintln(&b, s)
	// }

	b.WriteString("Solution: \n")
	totalCost := float32(0)
	for serverName, server := range s.GetServers() {
		srvClassName := server.GetServiceClassName()
		modelName := server.GetModelName()
		load := server.GetLoad()
		svc := GetServiceClass(srvClassName)
		if load == nil || svc == nil {
			continue
		}
		target := svc.GetModelTarget(modelName)
		if target == nil {
			continue
		}
		alloc := server.GetAllocation()
		if alloc == nil {
			fmt.Fprintf(&b, "s=%s; c=%s; m=%s; no feasible allocation! \n", serverName, srvClassName, modelName)
			continue
		}
		totalCost += alloc.cost
		rate := load.arrivalRate
		tokens := load.avgLength
		fmt.Fprintf(&b, "c=%s; m=%s; rate=%v; tk=%d; sol=%d, alloc=%v; ", srvClassName, modelName, rate, tokens, len(server.allAllocations), alloc)
		fmt.Fprintf(&b, "slo-itl=%v, slo-ttw=%v \n", target.ITL, target.TTW)
	}

	b.WriteString("AllocationByType: \n")
	for _, a := range s.allocationByType {
		fmt.Fprintf(&b, "%v \n", a)
	}
	fmt.Fprintf(&b, "totalCost=%v \n", totalCost)

	return b.String()
}
