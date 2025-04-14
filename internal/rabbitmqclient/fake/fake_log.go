package fake

import (
	logging "github.com/crossplane/crossplane-runtime/pkg/logging"
)

type MockLog struct {
}

func (l *MockLog) Info(msg string, keysAndValues ...any) {
}

func (l *MockLog) Debug(msg string, keysAndValues ...any) {}

func (l *MockLog) WithValues(keysAndValues ...any) logging.Logger {
	return l
}
