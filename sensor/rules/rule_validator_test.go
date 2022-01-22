package rules

import (
	"testing"
)

var (
	ruleMissingID = Rule{
		EventTypes: []EventType{ADDED},
	}
	ruleMissingEventsTypes = Rule{
		ID: "test-rule",
	}
	ruleInvalidEventTypes = Rule{
		ID:         "test-rule",
		EventTypes: []EventType{EventType("INVALID")},
	}
	ruleNormalized = Rule{
		ID:         "test-rule",
		EventTypes: []EventType{EventType("ADDED"), EventType("added"), ADDED},
	}
)

func TestRuleNormalizationAndValidation(t *testing.T) {
	ruleTestFunc := func(rule *Rule, expectedErr error) {
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
	if ruleNormalized.EventTypes[0] != ADDED {
		t.Fatal("Rule should have ADDED event type after normalization")
	}
}
