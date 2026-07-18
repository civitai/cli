package cmd

import "errors"

// ErrUsage classifies a command-line USAGE error — a bad flag or malformed
// invocation — as distinct from a runtime failure. It carries no visible text
// of its own: it is attached to Cobra's own flag-parsing error (see
// NewRootCmd's SetFlagErrorFunc) so that errors.Is(err, ErrUsage) is true while
// the message the user sees stays exactly what Cobra produced. The process
// entrypoint (cmd/civitai) maps it to a dedicated exit code.
var ErrUsage = errors.New("usage error")

// usageError attaches ErrUsage to a flag-parsing error WITHOUT altering the
// user-visible message: Error() delegates to the wrapped error while Unwrap()
// exposes both the original error and the ErrUsage sentinel.
type usageError struct{ err error }

func (u *usageError) Error() string   { return u.err.Error() }
func (u *usageError) Unwrap() []error { return []error{u.err, ErrUsage} }

// asUsageError tags err as a usage error (message preserved). A nil err is
// returned unchanged.
func asUsageError(err error) error {
	if err == nil {
		return nil
	}
	return &usageError{err: err}
}
