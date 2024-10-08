package core

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// A serverfor a service class and model
type Server struct {
	name             string
	serviceClassName string
	modelName        string

	// server load statistics
	load *ServerLoad

	// for all accelerators
	allAllocations map[string]*Allocation

	// allocated solution
	allocation *Allocation

	spec *config.ServerSpec
}

// request arrival and service statistics
type ServerLoad struct {
	arrivalRate float32 // req/min
	avgLength   int     // number of tokens
	arrivalCOV  float32 // coefficient of variation of inter-request arrival time
	serviceCOV  float32 // coefficient of variation of request service time
}

func (ld *ServerLoad) GetArrivalRate() float32 {
	return ld.arrivalRate
}

func (ld *ServerLoad) SetArrivalRate(a float32) {
	ld.arrivalRate = a
}

func (ld *ServerLoad) GetAvgLength() int {
	return ld.avgLength
}

func (ld *ServerLoad) SetAvgLength(l int) {
	ld.avgLength = l
}

func NewServerFromSpec(spec *config.ServerSpec) *Server {
	ld := &ServerLoad{
		arrivalRate: spec.ArrivalRate,
		avgLength:   spec.AvgLength,
		arrivalCOV:  spec.ArrivalCOV,
		serviceCOV:  spec.ServiceCOV,
	}
	return &Server{
		name:             spec.Name,
		serviceClassName: spec.Class,
		modelName:        spec.Model,
		load:             ld,
		allAllocations:   map[string]*Allocation{},
		allocation:       nil,
		spec:             spec,
	}
}

// Calculate allocations for all accelerators
func (s *Server) Calculate(accelerators map[string]*Accelerator) {
	s.allAllocations = make(map[string]*Allocation)
	for _, g := range accelerators {
		if alloc := CreateAllocation(s.name, g.name); alloc != nil {
			if curAlloc := s.allocation; curAlloc != nil {
				penalty := curAlloc.TransitionPenalty(alloc)
				alloc.SetValue(penalty)
			}
			s.allAllocations[g.name] = alloc
		}
	}
}

func (s *Server) GetName() string {
	return s.name
}

func (s *Server) GetServiceClassName() string {
	return s.serviceClassName
}

func (s *Server) GetModelName() string {
	return s.modelName
}

func (s *Server) GetLoad() *ServerLoad {
	return s.load
}

func (s *Server) GetAllocation() *Allocation {
	return s.allocation
}

func (s *Server) SetAllocation(alloc *Allocation) {
	s.allocation = alloc
}

func (s *Server) RemoveAllocation() {
	s.allocation = nil
}

func (s *Server) GetAllAllocations() map[string]*Allocation {
	return s.allAllocations
}

func (s *Server) GetSpec() *config.ServerSpec {
	return s.spec
}

func (s *Server) String() string {
	return fmt.Sprintf("Server: name=%s; class=%s; model=%s; load=%v; allocation=%v",
		s.name, s.serviceClassName, s.modelName, s.load, s.allocation)
}
