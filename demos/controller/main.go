package main

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/services/controller"
)

// create and run a Controller
func main() {
	ctrl, err := controller.NewController()
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := ctrl.Init(); err != nil {
		fmt.Println(err)
		return
	}
	ctrl.Run()
	controller.Wg.Wait()
}
