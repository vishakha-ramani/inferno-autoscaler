package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.ibm.com/tantawi/inferno/pkg/config"
	"github.ibm.com/tantawi/inferno/pkg/core"
	"github.ibm.com/tantawi/inferno/pkg/manager"
	"github.ibm.com/tantawi/inferno/pkg/solver"
)

var system *core.System

func main() {

	system = core.NewSystem()

	// populate system data from files (for testing only)
	size := "large"
	if len(os.Args) > 1 {
		size = os.Args[1]
	}
	prefix := "../samples/" + size + "/"
	fn_acc := prefix + "accelerator-data.json"
	fn_mod := prefix + "model-data.json"
	fn_svc := prefix + "serviceclass-data.json"
	fn_srv := prefix + "server-data.json"

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

	bytes_svc, err_svc := os.ReadFile(fn_svc)
	if err_svc != nil {
		fmt.Println(err_svc)
	}
	system.SetServiceClassesFromSpec(bytes_svc)

	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		fmt.Println(err_srv)
	}
	system.SetServersFromSpec(bytes_srv)
	// end populate system data

	// REST Server
	router := gin.Default()
	router.GET("/getAccelerators", getAccelerators)
	router.GET("/getAccelerator/:name", getAccelerator)
	router.POST("/addAccelerator", addAccelerator)
	router.GET("/removeAccelerator/:name", removeAccelerator)

	router.GET("/getCapacities", getCapacities)
	router.GET("/getCapacity/:type", getCapacity)
	router.POST("/addCapacity", addCapacity)
	router.GET("/removeCapacity/:type", removeCapacity)

	router.GET("/getModels", getModels)
	router.GET("/getModel/:name", getModel)
	router.POST("/addModel", addModel)
	router.GET("/removeModel/:name", removeModel)

	router.GET("/getServiceClasses", getServiceClasses)
	router.GET("/getServiceClass/:name", getServiceClass)
	router.GET("/addServiceClass/:name", addServiceClass)
	router.GET("/removeServiceClass/:name", removeServiceClass)

	router.GET("/getServiceClassModelTargets/:name/:model", getServiceClassModelTargets)
	router.POST("/addServiceClassModelTargets", addServiceClassModelTargets)
	router.GET("/removeServiceClassModelTargets/:name/:model", removeServiceClassModelTargets)

	router.GET("/getServers", getServers)
	router.GET("/getServer/:name", getServer)
	router.POST("/addServer", addServer)
	router.GET("/removeServer/:name", removeServer)

	router.GET("/getModelAcceleratorPerf/:name/:acc", getModelAcceleratorPerf)
	router.POST("/addModelAcceleratorPerf", addModelAcceleratorPerf)
	router.GET("/removeModelAcceleratorPerf/:name/:acc", removeModelAcceleratorPerf)

	router.POST("/optimize", optimize)

	router.Run("localhost:8080")
}

// Handlers
func getAccelerators(c *gin.Context) {
	accMap := system.GetAccelerators()
	gpus := make([]config.AcceleratorSpec, len(accMap))
	i := 0
	for _, acc := range accMap {
		gpus[i] = *acc.GetSpec()
		i++
	}
	c.IndentedJSON(http.StatusOK, gpus)
}

func getAccelerator(c *gin.Context) {
	name := c.Param("name")
	acc := system.GetAccelerator(name)
	if acc == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "accelerator " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, acc.GetSpec())
}

func addAccelerator(c *gin.Context) {
	var acc config.AcceleratorSpec
	if err := c.BindJSON(&acc); err != nil {
		return
	}
	system.AddAcceleratorFromSpec(acc)
	c.IndentedJSON(http.StatusOK, acc)
}

func removeAccelerator(c *gin.Context) {
	name := c.Param("name")
	acc := system.GetAccelerator(name)
	if err := system.RemoveAccelerator(name); err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "accelerator " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, acc.GetSpec())
}

func getCapacities(c *gin.Context) {
	capMap := system.GetCapacities()
	capacities := make([]config.AcceleratorCount, len(capMap))
	i := 0
	for k, v := range capMap {
		capacities[i] = config.AcceleratorCount{
			Type:  k,
			Count: v,
		}
		i++
	}
	c.IndentedJSON(http.StatusOK, capacities)
}

func getCapacity(c *gin.Context) {
	t := c.Param("type")
	cap, exists := system.GetCapacity(t)
	if !exists {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "capacity for " + t + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, config.AcceleratorCount{
		Type:  t,
		Count: cap,
	})
}

func addCapacity(c *gin.Context) {
	var count config.AcceleratorCount
	if err := c.BindJSON(&count); err != nil {
		return
	}
	system.AddCapacityFromSpec(count)
	cap, _ := system.GetCapacity(count.Type)
	c.IndentedJSON(http.StatusOK, config.AcceleratorCount{
		Type:  count.Type,
		Count: cap,
	})
}

func removeCapacity(c *gin.Context) {
	t := c.Param("type")
	cap, _ := system.GetCapacity(t)
	if !system.RemoveCapacity(t) {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "accelerator type " + t + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, config.AcceleratorCount{
		Type:  t,
		Count: cap,
	})
}

func getModels(c *gin.Context) {
	modelMap := system.GetModels()
	models := make([]config.ModelSpec, len(modelMap))
	i := 0
	for _, model := range modelMap {
		models[i] = *model.GetSpec()
		i++
	}
	c.IndentedJSON(http.StatusOK, models)
}

func getModel(c *gin.Context) {
	name := c.Param("name")
	model := system.GetModel(name)
	if model == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, model.GetSpec())
}

func addModel(c *gin.Context) {
	var model config.ModelSpec
	if err := c.BindJSON(&model); err != nil {
		return
	}
	system.AddModelFromSpec(model)
	c.IndentedJSON(http.StatusOK, model)
}

func removeModel(c *gin.Context) {
	name := c.Param("name")
	model := system.GetModel(name)
	if err := system.RemoveModel(name); err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, model.GetSpec())
}

func getServiceClasses(c *gin.Context) {
	svcMap := system.GetServiceClasses()
	svcs := make([]config.ServiceClassData, len(svcMap))
	i := 0
	for _, svc := range svcMap {
		svcs[i] = *svc.GetSpec()
		i++
	}
	c.IndentedJSON(http.StatusOK, svcs)
}

func getServiceClass(c *gin.Context) {
	name := c.Param("name")
	svc := system.GetServiceClass(name)
	if svc == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "service class " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, svc.GetSpec())
}

func addServiceClass(c *gin.Context) {
	name := c.Param("name")
	system.AddServiceClass(name)
	svc := system.GetServiceClass(name)
	c.IndentedJSON(http.StatusOK, svc.GetSpec())
}

func removeServiceClass(c *gin.Context) {
	name := c.Param("name")
	svc := system.GetServiceClass(name)
	if err := system.RemoveServiceClass(name); err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "service class " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, svc.GetSpec())
}

func getServiceClassModelTargets(c *gin.Context) {
	name := c.Param("name")
	model := c.Param("model")
	svc := system.GetServiceClass(name)
	if svc == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "service class " + name + " not found"})
		return
	}
	target := svc.GetModelTarget(model)
	if target == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + model + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, config.ServiceClassSpec{
		Name:    name,
		Model:   model,
		SLO_ITL: target.ITL,
		SLO_TTW: target.TTW,
	})
}

func addServiceClassModelTargets(c *gin.Context) {
	var targetSpec config.ServiceClassSpec
	if err := c.BindJSON(&targetSpec); err != nil {
		return
	}
	svcName := targetSpec.Name
	if system.GetServiceClass(svcName) == nil {
		system.AddServiceClass(svcName)
	}
	svc := system.GetServiceClass(svcName)
	svc.SetTargetFromSpec(&targetSpec)
	c.IndentedJSON(http.StatusOK, targetSpec)
}

func removeServiceClassModelTargets(c *gin.Context) {
	name := c.Param("name")
	model := c.Param("model")
	svc := system.GetServiceClass(name)
	if svc == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "service class " + name + " not found"})
		return
	}
	target := svc.GetModelTarget(model)
	if target == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + model + " not found"})
		return
	}
	svc.RemoveModelTarget(model)
	c.IndentedJSON(http.StatusOK, config.ServiceClassSpec{
		Name:    name,
		Model:   model,
		SLO_ITL: target.ITL,
		SLO_TTW: target.TTW,
	})
}

func getServers(c *gin.Context) {
	srvMap := system.GetServers()
	servers := make([]config.ServerSpec, len(srvMap))
	i := 0
	for _, server := range srvMap {
		servers[i] = *server.GetSpec()
		i++
	}
	c.IndentedJSON(http.StatusOK, servers)
}

func getServer(c *gin.Context) {
	name := c.Param("name")
	server := system.GetServer(name)
	if server == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "server " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, server.GetSpec())
}

func addServer(c *gin.Context) {
	var server config.ServerSpec
	if err := c.BindJSON(&server); err != nil {
		return
	}
	system.AddServerFromSpec(server)
	c.IndentedJSON(http.StatusOK, server)
}

func removeServer(c *gin.Context) {
	name := c.Param("name")
	server := system.GetServer(name)
	if err := system.RemoveServer(name); err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "server " + name + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, server.GetSpec())
}

func getModelAcceleratorPerf(c *gin.Context) {
	name := c.Param("name")
	acc := c.Param("acc")
	model := system.GetModel(name)
	if model == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + name + " not found"})
		return
	}
	perfData := model.GetPerfData(acc)
	if perfData == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "accelerator " + acc + " not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, perfData)
}

func addModelAcceleratorPerf(c *gin.Context) {
	var perfData config.ModelAcceleratorPerfData
	if err := c.BindJSON(&perfData); err != nil {
		return
	}
	modelName := perfData.Name
	model := system.GetModel(modelName)
	if model == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + modelName + " not found"})
		return
	}
	model.AddPerfDataFromSpec(&perfData)
	c.IndentedJSON(http.StatusOK, perfData)
}

func removeModelAcceleratorPerf(c *gin.Context) {
	name := c.Param("name")
	acc := c.Param("acc")
	model := system.GetModel(name)
	if model == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "model " + name + " not found"})
		return
	}
	perfData := model.GetPerfData(acc)
	if perfData == nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "accelerator " + acc + " not found"})
		return
	}
	model.RemovePerfData(acc)
	c.IndentedJSON(http.StatusOK, perfData)
}

func optimize(c *gin.Context) {
	var optimizerSpec config.OptimizerSpec
	if err := c.BindJSON(&optimizerSpec); err != nil {
		return
	}
	optimizer := solver.NewOptimizer(&optimizerSpec)
	manager := manager.NewManager(system, optimizer)
	system.Calculate()
	manager.Optimize()
	_, solution, err := system.GetSolution()
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "error marshaling solution"})
		return
	}
	c.IndentedJSON(http.StatusOK, solution)
}
