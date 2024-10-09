package main

import (
	"encoding/json"
	"fmt"
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
	fn_sol := prefix + "solution-data.json"

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

	allocationSolution := system.GenerateSolution()

	// generate json
	if byteValue, err := json.Marshal(allocationSolution); err != nil {
		fmt.Println(err)
	} else {
		os.WriteFile(fn_sol, byteValue, 0644)
	}

	fmt.Printf("%v", system)
	fmt.Printf("%v", optimizer)
}
