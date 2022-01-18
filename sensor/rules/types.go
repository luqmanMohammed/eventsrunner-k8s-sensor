package rules

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	ADDED    EventType = "ADDED"
	MODIFIED EventType = "MODIFIED"
	DELETED  EventType = "DELETED"
	NONE     EventType = "NONE"

	METADATA K8sObjectSubset = "metadata"
	SPEC     K8sObjectSubset = "spec"
	STATUS   K8sObjectSubset = "status"
)

//EventType is used to reperesent the type of event produced by the kubernetes api server
type EventType string

type K8sObjectSubset string

//Filter will be used to filter specific events based on labels and fields selectors
//All default kubernetes field selectors should work
type Filter struct {
	LabelFilter string `default:"" json:"labelFilter"`
	FieldFilter string `default:"" json:"fieldFilter"`
}

//Rule struct is used to represent a rule that will be used to filter events
type Rule struct {
	Filter
	schema.GroupVersionResource
	Name       string            `json:"Name"`
	Namespaces []string          `json:"namespaces"`
	EventTypes []EventType       `json:"eventTypes"`
	UpdatesOn  []K8sObjectSubset `json:"updatesOn"`
}
