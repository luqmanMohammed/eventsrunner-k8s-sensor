package validator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// RuleResourceIdentifierError is an error that occurs when there is an issue with a rule's resource identifier.
// Any one of group, version, or resource is invalid.
type RuleResourceIdentifierError struct {
	ResourceIdentifierType string
	ResourceIdentifier     string
}

// Error implements error interface.
func (r *RuleResourceIdentifierError) Error() string {
	return fmt.Sprintf("invalid resource identifier: %s: %s. no such %s in server", r.ResourceIdentifierType, r.ResourceIdentifier, r.ResourceIdentifierType)
}

var (
	// ErrRuleIDNotFound is returned when a rule doesn't have an ID.
	ErrRuleIDNotFound = errors.New("rule ID not found")
	// ErrRuleResourceIdentifiersNotFound is returned when a rule doesn't have any one
	// the felids required to identify a resource.
	ErrRuleResourceIdentifiersNotFound = errors.New("rule resource not valid. must have resource group, version, and resource")
	// ErrRuleEventTypesNotFound is returned when a rule doesn't have
	// an event types list.
	ErrRuleEventTypesNotFound = errors.New("rule event types not found")
	// ErrRuleEventTypesNotValid is returned when one or more event types
	// in rule is invalid.
	ErrRuleEventTypesNotValid = errors.New("rule event types not valid")
)

// NormalizeAndValidateRulesBatch normalizes and validates a map of rules.
// Normalized by making all event types lowercase and removing duplicates.
// Normalized by making all updateOn values lowercase and removing duplicates.
// Normalized by making all namespaces values lowercase and removing duplicates.
// Normalized by removing namespaces for cluster-bound resources.
// Validate rule ID is present.
// Validate resource identifiers are valid.
// Validate configured resources are part of the server's resources.
// Validate that all event types are one of added, modified, or deleted.
func NormalizeAndValidateRulesBatch(clientSet *kubernetes.Clientset, rulesMap map[rules.RuleID]*rules.Rule) (map[rules.RuleID]*rules.Rule, error) {
	klog.V(2).Infof("Validating a batch of rules: %d\n", len(rulesMap))
	serverResourceList, err := clientSet.DiscoveryClient.ServerPreferredResources()
	if err != nil {
		klog.V(1).ErrorS(err, "failed to get server resources. skipping rule normalization and validation")
		return nil, err
	}
	serverResources := make(map[string][]metav1.APIResource)
	for _, serverResourceGroup := range serverResourceList {
		serverResources[serverResourceGroup.GroupVersion] = serverResourceGroup.APIResources
	}
	normalizedRules := make(map[rules.RuleID]*rules.Rule)
	for id, rule := range rulesMap {
		normalizedRule, err := normalizeAndValidateRule(*rule, serverResources)
		if err != nil {
			klog.V(1).ErrorS(err, "failed to normalize and validate rule %s. this rule will be skipped", id)
			continue
		}
		normalizedRules[id] = &normalizedRule
	}
	return normalizedRules, nil
}

// normalizeAndValidateRule validates and normalizes a rule.
// Normalized by making all event types lowercase and removing duplicates.
// Normalized by making all updateOn values lowercase and removing duplicates.
func normalizeAndValidateRule(rule rules.Rule, serverResources map[string][]metav1.APIResource) (rules.Rule, error) {
	klog.V(3).Infof("Validating rule: %s", rule.ID)

	// Primary Validation
	if rule.ID == "" {
		return rules.Rule{}, ErrRuleIDNotFound
	}
	if len(rule.EventTypes) == 0 {
		return rules.Rule{}, ErrRuleEventTypesNotFound
	}
	if rule.Resource == "" || rule.Version == "" {
		return rules.Rule{}, ErrRuleResourceIdentifiersNotFound
	}

	// Normalization and secondary validation
	normalizedEventTypes, err := normalizeAndValidateEventTypes(rule.EventTypes)
	if err != nil {
		return rules.Rule{}, err
	}
	rule.EventTypes = normalizedEventTypes
	rule.UpdatesOn = normalizeUpdatesOn(rule.UpdatesOn)

	groupVersionStr := schema.GroupVersion{Group: rule.Group, Version: rule.Version}.String()
	resourceValid := false
	if resources, ok := serverResources[groupVersionStr]; ok {
		for _, resource := range resources {
			if resource.Name == rule.Resource {
				if len(rule.Namespaces) > 0 {
					if !resource.Namespaced {
						rule.Namespaces = []string{}
						klog.V(2).Infof("Resource in rule %s is not namespaced. Removing namespaces", rule.ID)
					} else {
						rule.Namespaced = true
						rule.Namespaces = normalizeNamespaces(rule.Namespaces)
					}
				}
				resourceValid = true
				break
			}
		}
	} else {
		return rules.Rule{}, &RuleResourceIdentifierError{
			ResourceIdentifierType: "group/version",
			ResourceIdentifier:     groupVersionStr,
		}
	}
	if !resourceValid {
		return rules.Rule{}, &RuleResourceIdentifierError{
			ResourceIdentifierType: "resource",
			ResourceIdentifier:     rule.Resource,
		}
	}
	return rule, nil
}

// normalizeAndValidateEventTypes validates and normalizes a list of event types.
// Normalized by making all event types lowercase and removing duplicates.
// Validate that all event types are one of added, modified, or deleted.
func normalizeAndValidateEventTypes(eventTypes []rules.EventType) ([]rules.EventType, error) {
	uniqueEventTypesSet := map[rules.EventType]struct{}{}
	for _, eventType := range eventTypes {
		lowerEventType := rules.EventType(strings.ToLower(string(eventType)))
		if _, ok := uniqueEventTypesSet[lowerEventType]; ok {
			continue
		}
		uniqueEventTypesSet[lowerEventType] = struct{}{}
	}
	normalizedEventTypes := make([]rules.EventType, 0, len(uniqueEventTypesSet))
	for eventType := range uniqueEventTypesSet {
		if eventType != rules.ADDED && eventType != rules.MODIFIED && eventType != rules.DELETED {
			return nil, ErrRuleEventTypesNotValid
		}
		normalizedEventTypes = append(normalizedEventTypes, eventType)
	}
	return normalizedEventTypes, nil
}

// normalizedNamespaces normalizes a list of namespace values.
// Normalized by making all namespaces values lowercase and removing duplicates.
func normalizeNamespaces(namespaces []string) []string {
	lowerNamespaces := utils.ConvertToStringLower(namespaces)
	return utils.RemoveDuplicateStrings(lowerNamespaces)
}

// normalizedUpdatesOn normalizes a list of updateOn values.
// Normalized by making all updateOn values lowercase and removing duplicates.
func normalizeUpdatesOn(updateOn []string) []string {
	lowerUpdatesOn := utils.ConvertToStringLower(updateOn)
	return utils.RemoveDuplicateStrings(lowerUpdatesOn)
}
