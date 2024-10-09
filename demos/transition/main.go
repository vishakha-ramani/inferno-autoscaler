package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"

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
		arv := load.ArrivalRate() * (1 + factorA)
		if arv <= 0 {
			arv = 1
		}
		load.SetArrivalRate(arv)

		factorB := 2 * (rand.Float32() - 0.5) * (1 - alpha)
		avl := int(math.Ceil(float64(float32(load.AvgLength()) * (1 + factorB))))
		if avl <= 0 {
			avl = 1
		}
		load.SetAvgLength(avl)

		// fmt.Printf("s=%s, rate=%v, tokens=%d \n",
		// 	server.GetName(), load.GetArrivalRate(), load.GetAvgLength())
	}

	system.Calculate()
	manager.Optimize()
	fmt.Printf("%v", system)
	fmt.Printf("%v", optimizer)
}
