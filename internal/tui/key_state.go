package tui

type apiKeyProviderOption struct {
	ID          string
	Label       string
	Description string
}

type keyEntryStep int

const (
	keyEntryProvider keyEntryStep = iota
	keyEntryAlias
	keyEntrySecret
)

type keyEntryState struct {
	Step           keyEntryStep
	ProviderCursor int
	Providers      []apiKeyProviderOption
	Provider       composerModel
	Alias          composerModel
	Secret         composerModel
}
