package main

import (
	"fmt"
	"os"

	core "github.ibm.com/tantawi/inferno/pkg/core"
)

func main() {
	prefix := "../../samples/"
	fn_acc := prefix + "accelerator-data.json"
	fn_mod := prefix + "model-data.json"
	fn_srv := prefix + "serviceclass-data.json"
	fn_opt := prefix + "optimizer-data.json"

	system := core.NewSystem()

	bytes_acc, err_acc := os.ReadFile(fn_acc)
	if err_acc != nil {
		fmt.Println(err_acc)
	}
	system.SetAcceleratorsFromSpec(bytes_acc)

	bytes_mod, err_mod := os.ReadFile(fn_mod)
	if err_mod != nil {
		fmt.Println(err_mod)
	}
	system.SetModelsFromSpec(bytes_mod)

	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		fmt.Println(err_srv)
	}
	system.SetServiceClassesFromSpec(bytes_srv)

	bytes_opt, err_opt := os.ReadFile(fn_opt)
	if err_opt != nil {
		fmt.Println(err_acc)
	}
	system.SetOptimizerFromSpec(bytes_opt)

	system.Calculate()
	system.Optimize()

	fmt.Printf("%v", system)
}
