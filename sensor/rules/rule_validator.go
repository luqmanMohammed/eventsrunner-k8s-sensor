package rules

import (
	"errors"
	"strings"
)

var (
	// ErrRuleIDNotFound is returned when a rule doesnt have an ID.
	ErrRuleIDNotFound = errors.New("rule ID not found")
	// ErrRuleEventTypesNotFound is returned when a rule doesnt have
	// an event types list.
	ErrRuleEventTypesNotFound = errors.New("rule event types not found")
	// ErrRuleEventTypesNotValid is returned when one or more event types
	// in rule is invalid.
	ErrRuleEventTypesNotValid = errors.New("rule event types not valid")
)

// NormalizeAndValidateRule validates and normalizes a rule.
// Normalized by making all event types lowercase and removing duplicates.
func NormalizeAndValidateRule(rule *Rule) error {
	if rule.ID == "" {
		return ErrRuleIDNotFound
	}
	if len(rule.EventTypes) == 0 {
		return ErrRuleEventTypesNotFound
	}
	uniqueEventTypesSet := map[EventType]struct{}{}
	normalizedEventTypes := make([]EventType, 0, len(rule.EventTypes))
	for _, eventType := range rule.EventTypes {
		lowerEventType := EventType(strings.ToLower(string(eventType)))
		if _, ok := uniqueEventTypesSet[lowerEventType]; ok {
			continue
		}
		normalizedEventTypes = append(normalizedEventTypes, lowerEventType)
		uniqueEventTypesSet[lowerEventType] = struct{}{}
	}
	rule.EventTypes = normalizedEventTypes
	for _, eventType := range rule.EventTypes {
		if eventType != ADDED && eventType != MODIFIED && eventType != DELETED {
			return ErrRuleEventTypesNotValid
		}
	}
	return nil
}
