package client

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/rest"
	"github.ibm.com/tantawi/inferno/pkg/utils"
)

// Controller is the main client user of the Optimizer:
//   - keeps static data about accelerators, models, and service classes
//   - updates dynamic data about servers through a Collector
//   - periodically calls the Optimizer to get servers desired state
//   - implements desired state through an Orchestrator
type Controller struct {
	State        *State
	Collector    *Collector
	Orchestrator *Orchestrator
}

type State struct {
	// all system data
	SystemData *config.SystemData

	// serialize operations on the state to keep it consistent
	sync.RWMutex
}

func NewController() *Controller {
	return &Controller{
		State: &State{
			SystemData: &config.SystemData{},
		},
		Collector:    NewCollector(),
		Orchestrator: NewOrchestrator(),
	}
}

// initialize
func (a *Controller) Init() error {
	state := a.State
	state.Lock()
	defer state.Unlock()

	// set URL to Optimizer REST server
	var host, port string
	if host = os.Getenv(rest.RestHostEnvName); host == "" {
		host = rest.DefaultRestHost
	}
	if port = os.Getenv(rest.RestPortEnvName); port == "" {
		port = rest.DefaultRestPort
	}
	OptimizerURL = "http://" + host + ":" + port

	// read static data from files in data path
	if DataPath = os.Getenv(DataPathEnvName); DataPath == "" {
		DataPath = DefaultDataPath
	}

	// read accelerator data
	fn_acc := DataPath + AcceleratorFileName
	bytes_acc, err_acc := os.ReadFile(fn_acc)
	if err_acc != nil {
		return err_acc
	}
	if d, err := utils.FromDataToSpec(bytes_acc, config.AcceleratorData{}); err == nil {
		state.SystemData.Spec.Accelerators = *d
	} else {
		return err
	}

	// read model data
	fn_mod := DataPath + ModelFileName
	bytes_mod, err_mod := os.ReadFile(fn_mod)
	if err_mod != nil {
		return err_mod
	}
	if d, err := utils.FromDataToSpec(bytes_mod, config.ModelData{}); err == nil {
		state.SystemData.Spec.Models = *d
	} else {
		return err
	}

	// read service class data
	fn_svc := DataPath + ServiceClassFileName
	bytes_svc, err_svc := os.ReadFile(fn_svc)
	if err_svc != nil {
		return err_svc
	}
	if d, err := utils.FromDataToSpec(bytes_svc, config.ServiceClassData{}); err == nil {
		state.SystemData.Spec.ServiceClasses = *d
	} else {
		return err
	}

	// read optimizer data
	fn_opt := DataPath + OptimizerFileName
	bytes_opt, err_opt := os.ReadFile(fn_opt)
	if err_opt != nil {
		return err_opt
	}
	if d, err := utils.FromDataToSpec(bytes_opt, config.OptimizerData{}); err == nil {
		state.SystemData.Spec.Optimizer = *d
	} else {
		return err
	}

	// initialize collector of dynamic server data
	if serverData, err := a.Collector.Init(); err != nil {
		return err
	} else {
		a.State.SystemData.Spec.Servers = *serverData
	}

	// prime REST server (for testing only!)
	if _, postErr := POSTOptimize(a.State.SystemData); postErr != nil {
		return postErr
	}
	return nil
}

// periodically run the controller
func (a *Controller) Run() {
	var controlPeriod int
	var err error
	if controlPeriod, err = strconv.Atoi(ControlPeriodSeconds); err != nil || controlPeriod <= 0 {
		controlPeriod = DefaultControlPeriodSeconds
	}

	// start periodic process
	go func() {
		agentTicker := time.NewTicker(time.Second * time.Duration(controlPeriod))
		for range agentTicker.C {
			if err := a.Optimize(); err != nil {
				fmt.Println(err)
				fmt.Println()
			}
		}
	}()
}

// run an optimization loop: update data, call optimizer, and orchestrate
func (a *Controller) Optimize() error {
	a.State.Lock()
	defer a.State.Unlock()

	// call Collector to get updated server data
	if err := a.Collector.Update(&a.State.SystemData.Spec.Servers); err != nil {
		return err
	}
	if len(a.State.SystemData.Spec.Servers.Spec) == 0 {
		return fmt.Errorf("missing server data")
	}

	// call optimizer
	if _, postErr := POSTOptimize(a.State.SystemData); postErr != nil {
		return postErr
	}

	// get server allocation solution and realize desired state
	if serverData, err := GetServerData(); err != nil {
		return err
	} else {
		a.Orchestrator.Do(serverData)
		a.State.SystemData.Spec.Servers = *serverData
		return nil
	}
}
