package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"

	"github.ibm.com/tantawi/inferno/pkg/config"
	core "github.ibm.com/tantawi/inferno/pkg/core"
	"github.ibm.com/tantawi/inferno/pkg/manager"
	"github.ibm.com/tantawi/inferno/pkg/solver"
)

func main() {
	size := "large"
	if len(os.Args) > 1 {
		size = os.Args[1]
	}
	prefix := "../../samples/" + size + "/"
	fn_acc := prefix + "accelerator-data.json"
	fn_mod := prefix + "model-data.json"
	fn_svc := prefix + "serviceclass-data.json"
	fn_srv := prefix + "server-data.json"
	fn_opt := prefix + "optimizer-data.json"

	system := core.NewSystem()

	bytes_acc, err_acc := os.ReadFile(fn_acc)
	if err_acc != nil {
		fmt.Println(err_acc)
	}
	system.SetAcceleratorsFromData(bytes_acc)

	bytes_mod, err_mod := os.ReadFile(fn_mod)
	if err_mod != nil {
		fmt.Println(err_mod)
	}
	system.SetModelsFromData(bytes_mod)

	bytes_svc, err_svc := os.ReadFile(fn_svc)
	if err_svc != nil {
		fmt.Println(err_svc)
	}
	system.SetServiceClassesFromData(bytes_svc)

	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		fmt.Println(err_srv)
	}
	system.SetServersFromData(bytes_srv)

	bytes_opt, err_opt := os.ReadFile(fn_opt)
	if err_opt != nil {
		fmt.Println(err_acc)
	}
	optimizer, err_opt := solver.NewOptimizerFromData(bytes_opt)
	if err_opt != nil {
		fmt.Println(err_acc)
	}

	manager := manager.NewManager(system, optimizer)

	system.Calculate()
	manager.Optimize()

	fmt.Printf("%v", system)
	fmt.Printf("%v", optimizer)

	// generate random values in [alpha, 2 - alpha), where 0 < alpha < 1
	alpha := float32(0.1)

	for _, server := range system.Servers() {
		load := server.Load()
		if load == nil {
			continue
		}

		factorA := 2 * (rand.Float32() - 0.5) * (1 - alpha)
		newArv := load.ArrivalRate * (1 + factorA)
		if newArv <= 0 {
			newArv = 1
		}

		factorB := 2 * (rand.Float32() - 0.5) * (1 - alpha)
		newLength := int(math.Ceil(float64(float32(load.AvgLength) * (1 + factorB))))
		if newLength <= 0 {
			newLength = 1
		}
		newLoad := config.ServerLoadSpec{
			ArrivalRate: newArv,
			AvgLength:   newLength,
			ArrivalCOV:  load.ArrivalCOV,
			ServiceCOV:  load.ServiceCOV,
		}
		server.SetLoad(&newLoad)
		if curAllocation := server.CurAllocation(); curAllocation != nil {
			server.SetCurAllocation(server.Allocation().Clone())
		}

		// fmt.Printf("s=%s, rate=%v, tokens=%d \n",
		// 	server.Name(), load.ArrivalRate, load.AvgLength)
	}

	system.Calculate()
	manager.Optimize()
	fmt.Printf("%v", system)
	fmt.Printf("%v", optimizer)
}
