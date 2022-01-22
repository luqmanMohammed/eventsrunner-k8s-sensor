package rules

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	ADDED    EventType = "added"
	MODIFIED EventType = "modified"
	DELETED  EventType = "deleted"
	NONE     EventType = "none"
)

// EventType is used to reperesent the type of event produced by the kubernetes api server
type EventType string

// Filter will be used to filter specific events based on labels and fields selectors
// All default kubernetes field selectors should work
type Filter struct {
	LabelFilter string `default:"" json:"labelFilter"`
	FieldFilter string `default:"" json:"fieldFilter"`
}

// RuleID unique identifier for a rule
type RuleID string

// Rule struct is used to represent a rule that will be used to filter events
type Rule struct {
	Filter
	schema.GroupVersionResource
	ID         RuleID      `json:"id"`
	Namespaces []string    `json:"namespaces"`
	EventTypes []EventType `json:"eventTypes"`
	UpdatesOn  []string    `json:"updatesOn"`
}
