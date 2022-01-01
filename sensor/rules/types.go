package rules

const (
	ADDED    EventType = "ADDED"
	MODIFIED EventType = "MODIFIED"
	DELETED  EventType = "DELETED"
	NONE     EventType = "NONE"
)

//EventType is used to reperesent the type of event produced by the kubernetes api server
type EventType string

//Filter will be used to filter specific events based on labels and fields selectors
//All default kubernetes field selectors should work
type Filter struct {
	LabelFilter string `default:""`
	FieldFilter string `default:""`
}

//Rule struct is used to represent a rule that will be used to filter events
type Rule struct {
	Filter
	Group      string
	APIVersion string
	Resource   string
	Namespaces []string
	EventTypes []EventType
}
