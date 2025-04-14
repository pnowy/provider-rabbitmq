package fake

import (
	"io"
)

type MockReadCloser struct {
}

func (l MockReadCloser) Close() error {
	return nil
}

func (l MockReadCloser) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}
