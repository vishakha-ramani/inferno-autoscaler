package collector

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.ibm.com/tantawi/inferno/pkg/config"
	ctrl "github.ibm.com/tantawi/inferno/services/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrometheusQuery executes a PromQL query and returns the result.
func PrometheusQuery(query string) (float64, error) {
	client, err := api.NewClient(api.Config{
		Address: "http://localhost:9090", // Update this with your Prometheus endpoint
	})
	if err != nil {
		return 0, fmt.Errorf("error creating Prometheus client: %v", err)
	}

	v1api := v1.NewAPI(client)
	result, warnings, err := v1api.Query(context.Background(), query, metav1.Now().Time)
	if err != nil {
		return 0, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		fmt.Printf("Prometheus query warnings: %v\n", warnings)
	}

	// Extract the result as a float64
	vector, ok := result.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, fmt.Errorf("no data returned from query")
	}
	return float64(vector[0].Value), nil
}

// Handlers for REST API calls

func collect(c *gin.Context) {
	// get managed deployments
	labelSelector := ctrl.KeyManaged + "=true"
	deps, err := KubeClient.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector})
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "kube client: " + err.Error()})
		return
	}

	// initialize collector info
	serverSpecs := make([]config.ServerSpec, 0)
	serverMap := make(map[string]ctrl.ServerKubeInfo)

	// collect data from deployments
	for _, d := range deps.Items {

		if d.ObjectMeta.Labels == nil || d.ObjectMeta.Labels[ctrl.KeyServerName] == "" {
			continue
		}
		serverName := d.ObjectMeta.Labels[ctrl.KeyServerName]

		depUID := string(d.UID)
		serverMap[serverName] = ctrl.ServerKubeInfo{
			UID:   depUID,
			Name:  d.ObjectMeta.Name,
			Space: d.ObjectMeta.Namespace,
		}

		numReplicas := *d.Spec.Replicas
		maxBatchSize, _ := strconv.Atoi(d.ObjectMeta.Labels[ctrl.KeyMaxBatchSize])
		// arrvRate, _ := strconv.ParseFloat(d.ObjectMeta.Labels[ctrl.KeyArrivalRate], 32)
		// avglength, _ := strconv.Atoi(d.ObjectMeta.Labels[ctrl.KeyNumTokens])

		var arrvRate float64 = 0.0
		var avglength float64 = 0.0

		// Query Prometheus for the arrival rate (requests/minute)
		arrivalQuery := fmt.Sprintf(`sum(rate(vllm:requests_count_total{job="%s"}[1m]))*60`, d.ObjectMeta.Name)
		arrivalRateFromPrometheus, err := PrometheusQuery(arrivalQuery)
		if err == nil {
			arrvRate = arrivalRateFromPrometheus
		} else {
			fmt.Println(err.Error())
		}
		fmt.Printf("Average arrival rate %f \n", arrvRate)

		// Query Prometheus for the token rate
		tokenQuery := fmt.Sprintf(`delta(vllm:tokens_count_total{job="%s"}[1m])/delta(vllm:requests_count_total{job="%s"}[1m])`, d.ObjectMeta.Name, d.ObjectMeta.Name)
		avgLengthFromPrometheus, err := PrometheusQuery(tokenQuery)
		if err == nil {
			avglength = avgLengthFromPrometheus
		} else {
			fmt.Println(err.Error())
		}
		fmt.Printf("Average token length per request %f \n", avglength)

		curAlloc := config.AllocationData{
			Accelerator: d.ObjectMeta.Labels[ctrl.KeyAccelerator],
			NumReplicas: int(numReplicas),
			MaxBatch:    maxBatchSize,
			Load: config.ServerLoadSpec{
				ArrivalRate: float32(arrvRate),
				AvgLength:   int(avglength),
			},
		}

		serverSpec := config.ServerSpec{
			Name:         serverName,
			Class:        d.ObjectMeta.Labels[ctrl.KeyServerClass],
			Model:        d.ObjectMeta.Labels[ctrl.KeyServerModel],
			CurrentAlloc: curAlloc,
		}
		serverSpecs = append(serverSpecs, serverSpec)
	}

	serverCollectorInfo := ctrl.ServerCollectorInfo{
		Spec:         serverSpecs,
		KubeResource: serverMap,
	}

	c.IndentedJSON(http.StatusOK, serverCollectorInfo)
}
