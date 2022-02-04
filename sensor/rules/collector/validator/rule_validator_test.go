package validator

import (
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
)

var (
	ruleMissingID = rules.Rule{
		EventTypes: []rules.EventType{rules.ADDED},
	}
	ruleMissingEventsTypes = rules.Rule{
		ID: "test-rule",
	}
	ruleInvalidEventTypes = rules.Rule{
		ID:         "test-rule",
		EventTypes: []rules.EventType{rules.EventType("INVALID")},
	}
	ruleNormalized = rules.Rule{
		ID:         "test-rule",
		EventTypes: []rules.EventType{rules.EventType("ADDED"), rules.EventType("added"), rules.ADDED},
	}
)

func TestRuleNormalizationAndValidation(t *testing.T) {
	ruleTestFunc := func(rule *rules.Rule, expectedErr error) {
		if err := NormalizeAndValidateRule(rule); err != expectedErr {
			t.Fatalf("Expected error %v got %v", expectedErr, err)
		}
	}
	ruleTestFunc(&ruleMissingID, ErrRuleIDNotFound)
	ruleTestFunc(&ruleMissingEventsTypes, ErrRuleEventTypesNotFound)
	ruleTestFunc(&ruleInvalidEventTypes, ErrRuleEventTypesNotValid)
	if err := NormalizeAndValidateRule(&ruleNormalized); err != nil {
		t.Fatal("Rule should be valid")
	}
	if len(ruleNormalized.EventTypes) != 1 {
		t.Fatal("Rule should have only one event type after normalization")
	}
	if ruleNormalized.EventTypes[0] != rules.ADDED {
		t.Fatal("Rule should have ADDED event type after normalization")
	}
}
