package validator

import (
	"errors"
	"strings"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/klog/v2"
)

var (
	// ErrRuleIDNotFound is returned when a rule doesn't have an ID.
	ErrRuleIDNotFound = errors.New("rule ID not found")
	// ErrRuleEventTypesNotFound is returned when a rule doesn't have
	// an event types list.
	ErrRuleEventTypesNotFound = errors.New("rule event types not found")
	// ErrRuleEventTypesNotValid is returned when one or more event types
	// in rule is invalid.
	ErrRuleEventTypesNotValid = errors.New("rule event types not valid")
)

// NormalizeAndValidateRule validates and normalizes a rule.
// Normalized by making all event types lowercase and removing duplicates.
// Normalized by making all updateOn values lowercase and removing duplicates.
func NormalizeAndValidateRule(rule *rules.Rule) error {
	klog.V(3).Infof("Validating rule: %s", rule.ID)
	if rule.ID == "" {
		return ErrRuleIDNotFound
	}
	if len(rule.EventTypes) == 0 {
		return ErrRuleEventTypesNotFound
	}
	uniqueEventTypesSet := map[rules.EventType]struct{}{}
	normalizedEventTypes := make([]rules.EventType, 0, len(rule.EventTypes))
	for _, eventType := range rule.EventTypes {
		lowerEventType := rules.EventType(strings.ToLower(string(eventType)))
		if _, ok := uniqueEventTypesSet[lowerEventType]; ok {
			continue
		}
		normalizedEventTypes = append(normalizedEventTypes, lowerEventType)
		uniqueEventTypesSet[lowerEventType] = struct{}{}
	}
	normalizedUpdateOnSet := map[string]struct{}{}
	normalizedUpdateOn := make([]string, 0, len(rule.UpdatesOn))
	for _, updateOn := range rule.UpdatesOn {
		lowerUpdateOn := strings.ToLower(string(updateOn))
		if _, ok := normalizedUpdateOnSet[lowerUpdateOn]; ok {
			continue
		}
		normalizedUpdateOn = append(normalizedUpdateOn, lowerUpdateOn)
		normalizedUpdateOnSet[lowerUpdateOn] = struct{}{}
	}
	rule.UpdatesOn = normalizedUpdateOn
	rule.EventTypes = normalizedEventTypes
	for _, eventType := range rule.EventTypes {
		if eventType != rules.ADDED && eventType != rules.MODIFIED && eventType != rules.DELETED {
			return ErrRuleEventTypesNotValid
		}
	}
	return nil
}
