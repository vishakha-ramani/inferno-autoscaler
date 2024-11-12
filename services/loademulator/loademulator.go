package loademulator

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"time"

	ctrl "github.ibm.com/tantawi/inferno/services/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	ArvRateRange   = [2]float32{6.0, 240.0}
	NumTokensRange = [2]int{100, 10000}
)

// Load emulator
type LoadEmulator struct {
	kubeClient *kubernetes.Clientset
	interval   time.Duration
	alpha      float32
}

// create a new load emulator
func NewLoadEmulator(intervalSec int, alpha float32) (loadEmulator *LoadEmulator, err error) {
	var kubeClient *kubernetes.Clientset
	if kubeClient, err = ctrl.GetKubeClient(); err == nil {
		return &LoadEmulator{
			kubeClient: kubeClient,
			interval:   time.Duration(intervalSec) * time.Second,
			alpha:      alpha,
		}, nil
	}
	return nil, err
}

// run the load emulator
func (lg *LoadEmulator) Run() {
	for {
		fmt.Println("Waiting " + lg.interval.String() + "...")
		time.Sleep(time.Duration(lg.interval))

		// get deployments
		labelSelector := ctrl.KeyManaged + "=true"
		deps, err := lg.kubeClient.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector})
		if err != nil {
			fmt.Println(err)
			continue
		}

		// update deployments
		for _, d := range deps.Items {
			curRPM64, _ := strconv.ParseFloat(d.Labels[ctrl.KeyArrivalRate], 32)
			curRPM := float32(curRPM64)
			curNumTokens, _ := strconv.Atoi(d.Labels[ctrl.KeyNumTokens])
			lg.perturbLoad(&curRPM, &curNumTokens)

			// update labels
			d.Labels[ctrl.KeyArrivalRate] = fmt.Sprintf("%.4f", curRPM)
			d.Labels[ctrl.KeyNumTokens] = fmt.Sprintf("%d", curNumTokens)
			if _, err := lg.kubeClient.AppsV1().Deployments(d.Namespace).Update(context.TODO(), &d, metav1.UpdateOptions{}); err != nil {
				fmt.Println(err)
				continue
			}
		}
		fmt.Printf("%d deployment(s) updated\n", len(deps.Items))
	}
}

// randomly modify dynamic server data (testing only)
func (lg *LoadEmulator) perturbLoad(rpm *float32, num *int) {
	// generate random values in [alpha, 2 - alpha), where 0 < alpha < 1
	factorA := 2 * (rand.Float32() - 0.5) * (1 - lg.alpha)
	newArv := *rpm * (1 + factorA)
	if newArv < ArvRateRange[0] {
		newArv = ArvRateRange[0]
	}
	if newArv > ArvRateRange[1] {
		newArv = ArvRateRange[1]
	}
	*rpm = newArv

	factorB := 2 * (rand.Float32() - 0.5) * (1 - lg.alpha)
	newLength := int(math.Ceil(float64(float32(*num) * (1 + factorB))))
	if newLength < NumTokensRange[0] {
		newLength = NumTokensRange[0]
	}
	if newLength > NumTokensRange[1] {
		newLength = NumTokensRange[1]
	}
	*num = newLength
}
