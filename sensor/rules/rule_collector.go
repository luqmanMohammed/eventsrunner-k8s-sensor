package rules

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ConfigMapRuleCollector is used to collect rules from a kubernetes configmap
// Rules will be collect from the configmaps with the label er-sensor-rules in
// the sensor namespace.
type ConfigMapRuleCollector struct {
	clientSet                *kubernetes.Clientset
	sensorNamespace          string
	sensorRuleConfigMapLabel string
}

// GetRules will return all rules from the configmaps in the sensor namespace
// with the label configured via sensorRuleConfigMapLabel.
// Only one copy of the duplicate rules will be returned.
// If the configmap does not exist or the rules are invalid, an empty map will
// be returned.
// If unable to list all configmaps in the sensor namespace, an error will be
// returned.
// TODO: Add rule validation
func (cmrc ConfigMapRuleCollector) Collect(ctx context.Context) (map[RuleID]*Rule, error) {
	rules := make(map[RuleID]*Rule)
	cmList, err := cmrc.clientSet.CoreV1().ConfigMaps(cmrc.sensorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: cmrc.sensorRuleConfigMapLabel,
	})

	if err != nil {
		return nil, err
	}

	for _, cm := range cmList.Items {
		tmpRules := []*Rule{}
		rulesStr, ok := cm.Data["rules"]
		if !ok {
			continue
		}
		if errJsonUnMarshal := json.Unmarshal([]byte(rulesStr), &tmpRules); errJsonUnMarshal != nil {
			continue
		}
		for _, rule := range tmpRules {
			if rule.ID == "" {
				continue
			}
			if _, ok := rules[rule.ID]; !ok {
				rules[rule.ID] = rule
			}
		}
	}
	return rules, nil
}
