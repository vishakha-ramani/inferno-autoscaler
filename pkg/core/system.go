package core

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// System comprising all accelerators, model, and service classes
type System struct {
	Accelerators   map[string]*Accelerator
	Models         map[string]*Model
	ServiceClasses map[string]*ServiceClass

	Optimizer *Optimizer

	capacity         map[string]int
	allocationByType map[string]*AllocationByType
}

type AllocationByType struct {
	name  string  // name of accelerator type
	count int     // total number of this type
	cost  float32 // total cost of this type
}

func NewSystem() *System {
	return &System{
		Accelerators:   make(map[string]*Accelerator),
		Models:         make(map[string]*Model),
		ServiceClasses: make(map[string]*ServiceClass),

		capacity:         make(map[string]int),
		allocationByType: map[string]*AllocationByType{},
	}
}

func (s *System) SetAcceleratorsFromSpec(byteValue []byte) error {
	var d config.AcceleratorData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for k, v := range d.Spec {
		s.Accelerators[k] = NewAcceleratorFromSpec(k, &v)
	}
	for _, v := range d.Count {
		s.capacity[v.Type] = v.Count
	}
	return nil
}

func (s *System) SetModelsFromSpec(byteValue []byte) error {
	var d config.ModelData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.Models[v.Name] = NewModelFromSpec(&v)
	}
	return nil
}

func (s *System) SetServiceClassesFromSpec(byteValue []byte) error {
	var d config.ServiceClassData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.ServiceClasses[v.Name] = NewServiceClassFromSpec(&v)
	}
	return nil
}

func (s *System) SetOptimizerFromSpec(byteValue []byte) error {
	var d config.OptimizerData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	s.Optimizer = NewOptimizerFromSpec(&d.Spec)
	return nil
}

func (s *System) GetAccelerator(name string) *Accelerator {
	return s.Accelerators[name]
}

// Calculate basic parameters
func (s *System) Calculate() {
	for _, g := range s.Accelerators {
		g.Calculate()
	}
	for _, m := range s.Models {
		m.Calculate(s.Accelerators)
	}
	for _, c := range s.ServiceClasses {
		c.Calculate(s.Models, s.Accelerators)
	}
}

func (s *System) Optimize() {
	s.Optimizer.Optimize(s)
}

// Accumulate allocation data by accelerator type
func (s *System) AllocateByType() {
	s.allocationByType = map[string]*AllocationByType{}
	for _, c := range s.ServiceClasses {
		for _, srvModelAlloc := range c.allocation {
			if srvModelAlloc == nil {
				continue
			}
			accName := srvModelAlloc.accelerator
			acc := s.Accelerators[accName]
			if acc == nil {
				continue
			}
			nameType := acc.GetType()
			var alloc *AllocationByType
			var exists bool
			if alloc, exists = s.allocationByType[nameType]; !exists {
				alloc = &AllocationByType{
					name:  nameType,
					count: 0,
					cost:  0,
				}
			}
			alloc.count += srvModelAlloc.numReplicas * acc.spec.Multiplicity
			alloc.cost += srvModelAlloc.cost
			s.allocationByType[nameType] = alloc
		}
	}
}

func (a *AllocationByType) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "name=%s, count=%d, cost=%v", a.name, a.count, a.cost)
	return b.String()
}

func (s *System) String() string {
	var b bytes.Buffer
	// b.WriteString("Accelerators: \n")
	// for _, g := range s.Accelerators {
	// 	fmt.Fprintln(&b, g)
	// }
	// b.WriteString("Models: \n")
	// for _, m := range s.models {
	// 	fmt.Fprintln(&b, m)
	// }
	// b.WriteString("ServiceClasses: \n")
	// for _, c := range s.serviceClasses {
	// 	fmt.Fprintln(&b, c)
	// }
	b.WriteString("Solution: \n")
	totalCost := float32(0)
	for _, c := range s.ServiceClasses {
		for i, v := range c.spec.Load {
			mName := v.Name
			alloc, exists := c.allocation[mName]
			if !exists {
				fmt.Fprintf(&b, "c=%s; m=%s; no feasible allocation! \n", c.spec.Name, mName)
				continue
			}
			totalCost += alloc.cost
			rate := c.modelLoad[mName].ArrivalRate
			tokens := c.modelLoad[mName].AvgLength
			fmt.Fprintf(&b, "c=%s; m=%s; rate=%v; tk=%d; sol=%d, alloc=%v; ", c.spec.Name, mName, rate, tokens, len(c.allAllocations[mName]), alloc)
			fmt.Fprintf(&b, "slo-itl=%v, slo-ttw=%v \n", c.spec.Load[i].SLO_ITL, c.spec.Load[i].SLO_TTW)
		}
	}

	b.WriteString("AllocationByType: \n")
	for _, a := range s.allocationByType {
		fmt.Fprintf(&b, "%v \n", a)
	}
	fmt.Fprintf(&b, "totalCost=%v \n", totalCost)

	return b.String()
}
