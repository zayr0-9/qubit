//go:build !linux

package platform

func NewNotifier() Notifier { return NoopNotifier{} }
