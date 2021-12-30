package sensor

import (
	"context"
	"testing"
	"time"

	"github.com/luqmanMohammed/er-k8s-sensor/common"
	"github.com/luqmanMohammed/er-k8s-sensor/sensor/rules"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	temp_rules_basic = []rules.Rule{
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "pods",
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
			Namespaces: []string{"default"},
		},
	}
	temp_rules_reload = []rules.Rule{
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "pods",
		},
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "pods",
		},
	}
)

func setupKubconfig() *rest.Config {
	if config, err := common.GetKubeAPIConfig(true, ""); err != nil {
		panic(err)
	} else {
		return config
	}
}
func setupSensor() *Sensor {
	config := setupKubconfig()
	sensor := New(&SensorOpts{
		KubeConfig:  config,
		SensorLabel: "k8s",
	})
	return sensor
}

func TestSensorStart(t *testing.T) {
	config := setupKubconfig()
	sensor := New(&SensorOpts{
		KubeConfig:  config,
		SensorLabel: "k8s",
	})
	go sensor.Start(&temp_rules_basic)
	defer close(sensor.StopChan)
	time.Sleep(3 * time.Second)
	if len(sensor.dynamicInformerFactories) != 1 {
		t.Error("Failed to start sensor")
	}
}

// func TestSensorReload(t *testing.T) {
// 	sensor := setupSensor()
// 	err := sensor.Start(&temp_rules_basic)
// 	sensor.ReloadRules(&temp_rules_reload)
// 	if len(sensor.dynamicInformerFactories) != 2 {
// 		t.Error("Failed to reload sensor")
// 	}
// }

func TestPodAdded(t *testing.T) {
	sensor := setupSensor()
	addEventTriggerd := false
	sensor.OverideEventFunctionOpts = &OverideEventFunctionOpts{
		AddFunc: func(obj interface{}) {
			if obj.(metav1.Object).GetName() == "test-pod" {
				addEventTriggerd = true
			}
		},
	}
	go sensor.Start(&temp_rules_basic)
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container",
					Image: "nginx",
				},
			},
		},
	}
	kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	for !addEventTriggerd {
		time.Sleep(1 * time.Second)
	}

}
