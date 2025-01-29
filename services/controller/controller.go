package controller

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/rest"
	"github.ibm.com/tantawi/inferno/pkg/utils"
)

var Wg sync.WaitGroup
var mutex sync.Mutex

var controller *Controller

// Controller is the main client user of the Optimizer:
//   - keeps static data about accelerators, models, and service classes
//   - updates dynamic data about servers through a Collector
//   - periodically calls the Optimizer to get servers desired state
//   - implements desired state through an Actuator
type Controller struct {
	State  *State
	router *gin.Engine
}

// State consists of static (read from files) and dynamic data
// (passed collector -> optimizer -> actuator). As such, static
// data maybe re-read after a potential crash. And, dynamic data
// does not need to persist beyond a control cycle. In case one
// or more of the three components fail/crash, the cycle is aborted.
type State struct {
	// all system data
	SystemData *config.SystemData

	ServerMap map[string]ServerKubeInfo
}

func NewController() (*Controller, error) {
	controller = &Controller{
		State: &State{
			SystemData: &config.SystemData{},
			ServerMap:  map[string]ServerKubeInfo{},
		},
		router: gin.Default(),
	}
	controller.router.GET("/invoke", invoke)
	return controller, nil
}

// initialize
func (a *Controller) Init() error {
	state := a.State

	CollectorURL = GetURL(CollectorHostEnvName, CollectorPortEnvName)
	OptimizerURL = GetURL(rest.RestHostEnvName, rest.RestPortEnvName)
	ActuatorURL = GetURL(ActuatorHostEnvName, ActuatorPortEnvName)

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

	// initialize dynamic server data
	state.SystemData.Spec.Servers = config.ServerData{
		Spec: make([]config.ServerSpec, 0),
	}

	return nil
}

// periodically run the controller
func (a *Controller) Run(controlPeriod int) {
	// start server
	Wg.Add(1)
	go func() {
		defer Wg.Done()
		host := ""
		port := "8080"
		if h := os.Getenv(ControllerHostEnvName); h != "" {
			host = h
		}
		if p := os.Getenv(ControllerPortEnvName); p != "" {
			port = p
		}
		a.router.Run(host + ":" + port)
	}()

	// start periodic process
	if controlPeriod > 0 {
		Wg.Add(1)
		go func() {
			defer Wg.Done()
			agentTicker := time.NewTicker(time.Second * time.Duration(controlPeriod))
			for range agentTicker.C {
				if err := a.Optimize(); err != nil {
					fmt.Printf("%v: skipping cycle ... reason=%s\n", time.Now().Format("15:04:05.000"), err.Error())
				}
			}
		}()
	}
}

// run an optimization loop: collect data, call optimizer, and actuate decisions
func (a *Controller) Optimize() error {
	mutex.Lock()
	defer mutex.Unlock()

	// call Collector to get updated server data
	startTime := time.Now()
	collectorInfo, collectErr := GETCollectorInfo()
	if collectErr != nil {
		return collectErr
	}
	if len(collectorInfo.Spec) == 0 {
		return fmt.Errorf("collector returned empty data")
	}
	a.State.SystemData.Spec.Servers.Spec = collectorInfo.Spec
	a.State.ServerMap = collectorInfo.KubeResource
	collectTime := time.Since(startTime)

	// call optimizer
	allocSolution, postErr := POSTOptimize(a.State.SystemData)
	if postErr != nil {
		return postErr
	}
	optimizeTime := time.Since(startTime) - collectTime

	// call Actuator to realize desired state
	actuatorInfo := &ServerActuatorInfo{
		Spec:         allocSolution.Spec,
		KubeResource: a.State.ServerMap,
	}
	actErr := POSTActuator(actuatorInfo)
	if actErr != nil {
		return actErr
	}
	actuateTime := time.Since(startTime) - collectTime - optimizeTime
	totalTime := time.Since(startTime)
	fmt.Printf("%v:\t collect: %d\t optimize: %d\t actuate: %d\t total: %d msec\n",
		time.Now().Format("15:04:05.000"),
		collectTime.Milliseconds(), optimizeTime.Milliseconds(),
		actuateTime.Milliseconds(), totalTime.Milliseconds())

	return nil
}
