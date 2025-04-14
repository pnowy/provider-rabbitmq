package fake

import (
	"io"
)

type MockReadCloser struct {
	MockClose func() (err error)
}

func (l MockReadCloser) Close() error {
	return l.MockClose()
}

func (l MockReadCloser) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}
