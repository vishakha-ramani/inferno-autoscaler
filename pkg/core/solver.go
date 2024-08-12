package core

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"slices"
)

// Solver of allocation assignment problem
type Solver struct {
	unlimited bool

	// current allocation for all service classes and models
	currentAllocation map[string]map[string]*Allocation

	// difference in allocation for all service classes and models
	diffAllocation map[string]map[string]*AllocationDiff
}

func NewSolver(unlimited bool) *Solver {
	return &Solver{
		unlimited:         unlimited,
		currentAllocation: make(map[string]map[string]*Allocation),
		diffAllocation:    make(map[string]map[string]*AllocationDiff),
	}
}

// Entry in the solution space for a service class and model pair
type entry struct {
	sName       string        // service class
	mName       string        // model name
	curIndex    int           // current index in allocation list
	allocations []*Allocation // ordered list of allocations
	delta       float32       // delta penalty if current allocation not allowed and next allocation is allowed
}

// Find optimal allocation for all service classes
func (s *Solver) Solve(system *System) {
	// take snapshot of current allocations
	s.currentAllocation = make(map[string]map[string]*Allocation)
	for srvClassName, sc := range system.serviceClasses {
		s.currentAllocation[srvClassName] = make(map[string]*Allocation)
		if sc.allocation == nil {
			continue
		}
		for modelName, alloc := range sc.allocation {
			s.currentAllocation[srvClassName][modelName] = alloc.Clone()
		}
	}
	// find solution
	if s.unlimited {
		s.SolveUnlimited(system)
	} else {
		s.SolveLimited(system)
	}
	// calculate difference
	s.diffAllocation = make(map[string]map[string]*AllocationDiff)
	for srvClassName, sc := range system.serviceClasses {
		s.diffAllocation[srvClassName] = make(map[string]*AllocationDiff)
		curMapModel := s.currentAllocation[srvClassName]
		mapModel := sc.allocation
		for modelName := range system.GetModels() {
			if allocDiff := CreateAllocationDiff(curMapModel[modelName], mapModel[modelName]); allocDiff != nil {
				s.diffAllocation[srvClassName][modelName] = allocDiff
			}
		}
	}
}

// Find optimal allocations assuming unlimited accelerator capacity
func (s *Solver) SolveUnlimited(system *System) {
	for _, v := range system.GetServiceClasses() {
		// select allocation with minimum value
		for mName, modelMap := range v.allAllocations {
			minVal := float32(math.MaxFloat32)
			var minAlloc *Allocation
			for _, alloc := range modelMap {
				if alloc.value < minVal {
					minVal = alloc.value
					minAlloc = alloc
				}
			}
			if minAlloc != nil {
				v.SetAllocation(mName, minAlloc)
			} else {
				v.RemoveAllocation(mName)
			}
		}
	}
}

// Find optimal allocations assuming limited accelerator capacity
func (s *Solver) SolveLimited(system *System) {
	// calculate available count of accelerator types
	available := make(map[string]int)
	for k := range system.capacity {
		available[k] = system.capacity[k]
	}
	// for all service classes and models, sort allocations
	var entries []*entry = make([]*entry, 0)
	for srvClassName, sc := range system.GetServiceClasses() {
		for modelName, modelMap := range sc.allAllocations {
			e := &entry{
				sName:       srvClassName,
				mName:       modelName,
				curIndex:    0,
				allocations: make([]*Allocation, len(modelMap)),
				delta:       0,
			}
			i := 0
			for _, alloc := range modelMap {
				e.allocations[i] = alloc
				i++
			}
			slices.SortFunc(e.allocations, func(a, b *Allocation) int {
				return cmp.Compare(a.value, b.value)
			})
			if len(e.allocations) > 1 {
				// value is difference between this and next allocation
				e.delta = e.allocations[1].value - e.allocations[0].value
			} else {
				// last choice, large value for not assigning
				e.delta = math.MaxFloat32
			}
			entries = append(entries, e)
		}
	}
	// sort all entries
	orderFunc := func(a, b *entry) int {
		if a.delta == b.delta {
			return cmp.Compare(b.allocations[b.curIndex].value, a.allocations[a.curIndex].value)
		}
		return cmp.Compare(b.delta, a.delta)
	}
	slices.SortFunc(entries, orderFunc)
	// start assignment greedily
	for len(entries) > 0 {
		top := entries[0]
		entries = entries[1:]

		if len(top.allocations) == 0 {
			continue
		}
		alloc := top.allocations[top.curIndex]
		gName := alloc.accelerator
		replicas := alloc.numReplicas
		acc := system.GetAccelerator(gName)
		tName := acc.GetType()
		count := replicas * acc.spec.Multiplicity

		if available[tName] >= count {
			available[tName] -= count
			c := system.GetServiceClass(top.sName)
			c.SetAllocation(top.mName, alloc)
		} else {
			top.curIndex++
			if top.curIndex+1 < len(top.allocations) {
				top.delta = top.allocations[top.curIndex+1].value - top.allocations[top.curIndex].value
			} else if top.curIndex == len(top.allocations) {
				continue
			} else {
				top.delta = math.MaxFloat32
			}
			i, _ := slices.BinarySearchFunc(entries, top, orderFunc)
			entries = slices.Insert(entries, i, top)
		}
	}
}

func (s *Solver) GetAllocationDiff() map[string]map[string]*AllocationDiff {
	return s.diffAllocation
}

func (e *entry) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "sName=%s, mName=%s, curIndex=%d, delta=%v, allocations=%v \n",
		e.sName, e.mName, e.curIndex, e.delta, e.allocations)
	return b.String()
}

func (s *Solver) String() string {
	var b bytes.Buffer
	b.WriteString("Solver: \n")
	for srvName, modelDiffMap := range s.diffAllocation {
		for modelName, allocDiff := range modelDiffMap {
			fmt.Fprintf(&b, "sName=%s, mName=%s, allocDiff=%v \n",
				srvName, modelName, allocDiff)
		}
	}
	return b.String()
}
