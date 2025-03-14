package main

import (
	"fmt"

	"github.ibm.com/ai-platform-optimization/inferno/services/actuator"
)

// create and run an Actuator server
func main() {
	actuator, err := actuator.NewActuator()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	actuator.Run()
}
