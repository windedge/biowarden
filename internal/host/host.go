// Package host is the native messaging host: the browser launches the binary
// with pipes on stdin and stdout and the host serves the extension until they
// close. An unlock reply carries key material, so its logging is redacted.
package host

import (
	"errors"
	"io"
)

var ErrNotImplemented = errors.New("host: serve loop not implemented yet")

// Run serves frames from stdin, answering on stdout, until the browser closes
// stdin.
func Run(stdin io.Reader, stdout io.Writer) error {
	return ErrNotImplemented
}
