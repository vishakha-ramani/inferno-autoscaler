package main

import (
	"fmt"

	"github.com/llm-inferno/inferno/services/collector"
)

// create and run a Collector server
func main() {
	collector, err := collector.NewCollector()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	collector.Run()
}
