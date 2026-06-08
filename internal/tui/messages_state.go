package tui

const (
	messageKindStatus     = "status"
	messageKindReminder   = "reminder"
	messageKindCompaction = "compaction"
)

type queuedMessageKind string

const (
	queuedMessageStatus   queuedMessageKind = "status"
	queuedMessageReminder queuedMessageKind = "reminder"
	queuedMessageUser     queuedMessageKind = "user"
)

type queuedMessage struct {
	Kind        queuedMessageKind
	Role        string
	Content     string
	SendToModel bool
}
