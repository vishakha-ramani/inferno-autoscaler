package solver

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/core"
)

// Solver of allocation assignment problem
type Solver struct {
	optimizerSpec *config.OptimizerSpec

	// current allocation for all servers
	currentAllocation map[string]*core.Allocation

	// difference in allocation for all servers
	diffAllocation map[string]*core.AllocationDiff
}

func NewSolver(optimizerSpec *config.OptimizerSpec) *Solver {
	return &Solver{
		optimizerSpec:     optimizerSpec,
		currentAllocation: make(map[string]*core.Allocation),
		diffAllocation:    make(map[string]*core.AllocationDiff),
	}
}

// Entry in the solution space for a service class and model pair
type entry struct {
	serverName  string             // server name
	curIndex    int                // current index in allocation list
	allocations []*core.Allocation // ordered list of allocations
	delta       float32            // delta penalty if current allocation not allowed and next allocation is allowed
}

func (e *entry) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "sName=%s, curIndex=%d, delta=%v, allocations=%v \n",
		e.serverName, e.curIndex, e.delta, e.allocations)
	return b.String()
}

// Find optimal allocation for all service classes
func (s *Solver) Solve() {
	// take snapshot of current allocations
	s.currentAllocation = make(map[string]*core.Allocation)
	for serverName, server := range core.GetServers() {
		if alloc := server.CurAllocation(); alloc != nil {
			s.currentAllocation[serverName] = alloc
		}
	}

	// find solution
	if s.optimizerSpec.MILPSolver {
		s.SolveMILP()
	} else if s.optimizerSpec.Unlimited {
		s.SolveUnlimited()
	} else {
		s.SolveLimited()
	}
	// calculate difference

	// TODO: cleanup after trying MIP solver

	s.diffAllocation = make(map[string]*core.AllocationDiff)
	for serverName, server := range core.GetServers() {
		curAlloc := s.currentAllocation[serverName]
		desiredAlloc := server.Allocation()
		if allocDiff := core.CreateAllocationDiff(curAlloc, desiredAlloc); allocDiff != nil {
			s.diffAllocation[serverName] = allocDiff
		}
	}

}

// Find optimal allocations assuming unlimited accelerator capacity
func (s *Solver) SolveUnlimited() {
	for _, server := range core.GetServers() {
		// select allocation with minimum value
		minVal := float32(math.MaxFloat32)
		var minAlloc *core.Allocation
		for _, alloc := range server.AllAllocations() {
			if alloc.Value() < minVal {
				minVal = alloc.Value()
				minAlloc = alloc
			}
		}
		if minAlloc != nil {
			server.SetAllocation(minAlloc)
		} else {
			server.RemoveAllocation()
		}
	}
}

// Find optimal allocations assuming limited accelerator capacity
func (s *Solver) SolveLimited() {
	// calculate available count of accelerator types
	available := make(map[string]int)
	for k, v := range core.GetCapacities() {
		available[k] = v
	}
	// for all servers, sort allocations
	var entries []*entry = make([]*entry, 0)
	for serverName, server := range core.GetServers() {
		allAllocs := server.AllAllocations()
		e := &entry{
			serverName:  serverName,
			curIndex:    0,
			allocations: make([]*core.Allocation, len(allAllocs)),
			delta:       0,
		}
		i := 0
		for _, alloc := range allAllocs {
			e.allocations[i] = alloc
			i++
		}
		slices.SortFunc(e.allocations, func(a, b *core.Allocation) int {
			return cmp.Compare(a.Value(), b.Value())
		})
		if len(e.allocations) > 1 {
			// value is difference between this and next allocation
			e.delta = e.allocations[1].Value() - e.allocations[0].Value()
		} else {
			// last choice, large value for not assigning
			e.delta = math.MaxFloat32
		}
		entries = append(entries, e)
	}
	// sort all entries
	orderFunc := func(a, b *entry) int {
		if a.delta == b.delta {
			return cmp.Compare(b.allocations[b.curIndex].Value(), a.allocations[a.curIndex].Value())
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

		serverName := top.serverName
		server := core.GetServer(serverName)
		if server == nil {
			continue
		}
		model := core.GetModel(server.ModelName())
		if model == nil {
			continue
		}

		alloc := top.allocations[top.curIndex]
		gName := alloc.Accelerator()
		replicas := alloc.NumReplicas()
		acc := core.GetAccelerator(gName)
		tName := acc.Type()
		count := replicas * model.NumInstances(gName) * acc.Spec().Multiplicity

		if available[tName] >= count {
			available[tName] -= count
			server := core.GetServer(serverName)
			server.SetAllocation(alloc)
		} else {
			top.curIndex++
			if top.curIndex+1 < len(top.allocations) {
				top.delta = top.allocations[top.curIndex+1].Value() - top.allocations[top.curIndex].Value()
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

func (s *Solver) SolveMILP() {
	mip := NewMILPSolver(s.optimizerSpec)
	mip.Solve()
}

func (s *Solver) AllocationDiff() map[string]*core.AllocationDiff {
	return s.diffAllocation
}

func (s *Solver) String() string {
	var b bytes.Buffer
	b.WriteString("Solver: \n")
	for serverName, allocDiff := range s.diffAllocation {
		fmt.Fprintf(&b, "sName=%s, allocDiff=%v \n",
			serverName, allocDiff)
	}
	return b.String()
}
