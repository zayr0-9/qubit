package tui

type modalKind string

const (
	modalKindPermission modalKind = "permission"
	modalKindConfirm    modalKind = "confirm"
	modalKindQuestion   modalKind = "question"
	modalKindCustom     modalKind = "custom"
)

type modalAction struct {
	ID      string
	Label   string
	Style   string
	Default bool
}

type modalField struct {
	Label string
	Value string
}
type modalOption struct {
	ID          string
	Label       string
	Description string
	Active      bool
}
