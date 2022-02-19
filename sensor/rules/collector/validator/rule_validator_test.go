package validator

import (
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

var (
	ruleMissingID = rules.Rule{
		EventTypes: []rules.EventType{rules.ADDED},
	}
	ruleMissingEventsTypes = rules.Rule{
		ID: "test-rule-missing-events-types",
	}
	ruleInvalidEventTypes = rules.Rule{
		ID:                   "test-rule-invalid-event-types",
		GroupVersionResource: schema.GroupVersionResource{Group: "group", Version: "version", Resource: "resource"},
		EventTypes:           []rules.EventType{rules.EventType("INVALID")},
	}
	ruleVersionMissingResource = rules.Rule{
		ID:                   "test-rule-version-missing-resource",
		GroupVersionResource: schema.GroupVersionResource{Group: "group", Version: "version", Resource: ""},
		EventTypes:           []rules.EventType{rules.ADDED},
	}
	ruleInvalidGroup = rules.Rule{
		ID:                   "test-rule-invalid-group",
		GroupVersionResource: schema.GroupVersionResource{Group: "group", Version: "v1", Resource: "resource"},
		EventTypes:           []rules.EventType{rules.ADDED},
	}
	ruleInvalidResource = rules.Rule{
		ID:                   "test-rule-invalid-resource",
		GroupVersionResource: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "resource"},
		EventTypes:           []rules.EventType{rules.ADDED},
	}
	ruleNormalized = rules.Rule{
		ID:                   "test-rule-normalized",
		GroupVersionResource: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		EventTypes:           []rules.EventType{rules.EventType("ADDED"), rules.EventType("added"), rules.ADDED},
		UpdatesOn:            []string{"metadata", "METADATA", "spec"},
		Namespaces:           []string{"default", "kube-system"},
	}
	ruleNormalizedNamespace = rules.Rule{
		ID:                   "test-rule-normalized-namespace",
		GroupVersionResource: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"},
		EventTypes:           []rules.EventType{rules.EventType("ADDED")},
		Namespaces:           []string{"default", "kube-system"},
	}
)

func TestRuleNormalizationAndValidation(t *testing.T) {
	clientset := kubernetes.NewForConfigOrDie(utils.GetKubeAPIConfigOrDie(""))
	testRulesMap := map[rules.RuleID]rules.Rule{
		ruleMissingID.ID:              ruleMissingID,
		ruleMissingEventsTypes.ID:     ruleMissingEventsTypes,
		ruleInvalidEventTypes.ID:      ruleInvalidEventTypes,
		ruleVersionMissingResource.ID: ruleVersionMissingResource,
		ruleInvalidGroup.ID:           ruleInvalidGroup,
		ruleInvalidResource.ID:        ruleInvalidResource,
		ruleNormalized.ID:             ruleNormalized,
		ruleNormalizedNamespace.ID:    ruleNormalizedNamespace,
	}
	normalizedRulesMap := NormalizeAndValidateRulesBatch(clientset, testRulesMap)
	if normalizedRulesMap == nil {
		t.Fatalf("normalized rules map is nil")
	}
	if len(normalizedRulesMap) != 2 {
		t.Fatalf("normalized rules map has %d rules", len(normalizedRulesMap))
	}
	if nsNormRule, ok := normalizedRulesMap[ruleNormalizedNamespace.ID]; !ok {
		t.Fatalf("normalized rules map does not contain normalized namespace rule")
	} else {
		if len(nsNormRule.Namespaces) > 0 {
			t.Fatalf("normalized namespace rule has %d namespaces", len(nsNormRule.Namespaces))
		}
	}
}
