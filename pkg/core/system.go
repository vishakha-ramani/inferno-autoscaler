package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type System struct {
	accelerators   map[string]*Accelerator
	models         map[string]*Model
	capacity       map[string]int
	serviceClasses map[string]*ServiceClass

	allocationByType map[string]*AllocationByType
}

type AllocationByType struct {
	name  string
	count int
	cost  float32
}

func NewSystem() *System {
	return &System{
		accelerators:   make(map[string]*Accelerator),
		models:         make(map[string]*Model),
		capacity:       make(map[string]int),
		serviceClasses: make(map[string]*ServiceClass),

		allocationByType: map[string]*AllocationByType{},
	}
}

func (s *System) SetAccelerators(byteValue []byte) error {
	var d AcceleratorData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for k, v := range d.Spec {
		s.accelerators[k] = NewAcceleratorFromSpec(k, &v)
	}
	for _, v := range d.Count {
		s.capacity[v.Type] = v.Count
	}
	return nil
}

func (s *System) SetModels(byteValue []byte) error {
	var d ModelData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.models[v.Name] = NewModelFromSpec(&v)
	}
	return nil
}

func (s *System) SetServiceClasses(byteValue []byte) error {
	var d ServiceClassData
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return err
	}
	for _, v := range d.Spec {
		s.serviceClasses[v.Name] = NewServiceClassFromSpec(&v)
	}
	return nil
}

func (s *System) GetAccelerator(name string) *Accelerator {
	return s.accelerators[name]
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

func (s *System) AllocateByType() {
	for _, c := range s.serviceClasses {
		for _, srvModelAlloc := range c.allocation {
			accName := srvModelAlloc.Accelerator
			acc := s.accelerators[accName]
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
			alloc.count += srvModelAlloc.NumReplicas * acc.Spec.Multiplicity
			alloc.cost += srvModelAlloc.Cost
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
	b.WriteString("Accelerators: \n")
	for _, g := range s.accelerators {
		fmt.Fprintln(&b, g)
	}
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
	for _, c := range s.serviceClasses {
		for i, v := range c.Spec.ModelLoad {
			mName := v.Name
			alloc, exists := c.allocation[mName]
			if !exists {
				fmt.Fprintf(&b, "c=%s; m=%s; no feasible allocation! \n", c.Spec.Name, mName)
				continue
			}
			totalCost += alloc.Cost
			fmt.Fprintf(&b, "c=%s; m=%s; choices=%d, a=%v; ", c.Spec.Name, mName, len(c.AllAllocations[mName]), alloc)
			fmt.Fprintf(&b, "slo-itl=%v, slo-ttw=%v \n", c.Spec.ModelLoad[i].SLO_ITL, c.Spec.ModelLoad[i].SLO_TTW)
		}
	}

	b.WriteString("AllocationByType: \n")
	for _, a := range s.allocationByType {
		fmt.Fprintf(&b, "%v \n", a)
	}
	fmt.Fprintf(&b, "totalCost=%v \n", totalCost)

	return b.String()
}
