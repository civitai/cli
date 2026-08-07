package civitai

import (
	"io/fs"
	"net"
	"os"
	"syscall"
)

// 🔴 A FILESYSTEM ERROR IS NOT A NETWORK ERROR, and the standard library makes
// that startlingly easy to get wrong. `syscall.Errno` carries both
// `Timeout() bool` and `Temporary() bool`, so it satisfies `net.Error` — while
// `*fs.PathError`, `*os.LinkError` and `*os.SyscallError` do NOT (measured on
// go1.25.12: none of the three declares `Temporary()`). A plain
// `errors.As(err, &netErr)` therefore unwraps straight PAST the filesystem
// wrapper and matches the bare Errno underneath it.
//
// That single spelling was wrong in TWO places — `exitCode`'s classifier in
// cmd/civitai (issue #241, fixed by #242) and the read-GET retry loop in this
// package (issue #244). One rule, one place: the filesystem-exclusion walk now
// lives here and both callers ask it, because a predicate open-coded at two
// sites is how the second copy survived the first fix.
//
// The two callers deliberately do NOT want the same ANSWER — `exitCode` asks
// "is this exit 5?", `isTransientNetErr` asks "should I retry?", and those
// differ (context.Canceled is never retried but is not a filesystem error;
// a *net.DNSError that is not a timeout is exit 5 but is not retried). So what
// is shared is the EVIDENCE — which net.Error, if any, the net stack
// contributed — and each caller draws its own conclusion from it.

// transportError returns the net.Error that the NET STACK contributed to err's
// tree, if there is one. It walks both the `Unwrap() error` and `Unwrap() []error`
// shapes.
//
// It is close to, but deliberately not, `errors.As(err, &netErr)`. Two rules,
// and neither subsumes the other:
//
//   - A bare `syscall.Errno` is not evidence. It satisfies net.Error by
//     accident of having Timeout()/Temporary(); the walk SKIPS it and keeps
//     going rather than stopping, so a genuine net.Error elsewhere in a
//     multi-error tree is still found. The skip is a type ASSERTION on the
//     matched value, never `errors.As`, because `errors.As` unwraps a
//     *net.OpError down to its ECONNREFUSED and would reject every real dial
//     failure.
//   - An error ABOUT A PATH is a filesystem error, full stop. `*fs.PathError`
//     and `*os.LinkError` terminate the walk, so nothing beneath one can be
//     read as transport evidence. Today that is belt-and-braces (they bottom
//     out in an Errno the first rule already rejects); it is here because a
//     future Go release adding `Temporary()` to either type would otherwise
//     re-open this exact hole silently, matching the wrapper itself instead of
//     the Errno. No net API returns either type.
//
// `*os.SyscallError` is deliberately NOT a terminator: the net stack nests one
// inside *net.OpError on every dial failure, and the OpError — a real
// net.Error — is found before the walk ever reaches it.
//
// 🔴 It does NOT "see everything errors.As would", and the earlier wording in
// cmd/civitai saying so was simply wrong. This walk consults only the two Unwrap
// shapes; `errors.As` ALSO calls a matched type's own `As(any) bool` method, and
// a type that implements one can hand errors.As a target the walk never reaches.
// Measured on this module: zero `func (…) As(any) bool` / `As(interface{}) bool`
// declarations in the repo and across all 119 dependency package directories
// (1445 .go files scanned; the scanner's pattern was positive-controlled against
// both spellings), so no error tree reachable from this CLI is classified
// differently today. It is unreachable, not equivalent — a dependency that adds
// an `As` method could hide a net.Error from this walk.
func transportError(err error) (net.Error, bool) {
	for err != nil {
		switch err.(type) {
		case *fs.PathError, *os.LinkError:
			return nil, false
		}
		if ne, ok := err.(net.Error); ok {
			if _, isErrno := ne.(syscall.Errno); !isErrno {
				return ne, true
			}
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, e := range x.Unwrap() {
				if ne, ok := transportError(e); ok {
					return ne, true
				}
			}
			return nil, false
		default:
			return nil, false
		}
	}
	return nil, false
}

// IsTransportError reports whether err is (or wraps) a transport failure that
// the NET STACK contributed — a *net.OpError, *net.DNSError, *url.Error, a
// deadline, and so on — as opposed to a filesystem failure whose bare
// `syscall.Errno` merely satisfies net.Error by accident. See transportError
// for the two rules and why they are not `errors.As`.
//
// It is the thin exported seam over that walk, and exists so `cmd/civitai`'s
// exit-code classifier and this package's retry loop share ONE copy of the
// filesystem-exclusion rule rather than two. It answers only the presence
// question; a caller that needs the matched error's Timeout() (the retry loop
// does) uses the unexported transportError, so the public surface stays one
// boolean rather than a net.Error the SDK would then owe compatibility on.
//
// It is NOT a retry predicate: a *net.DNSError for a host that does not exist is
// a transport error and is not worth retrying.
func IsTransportError(err error) bool {
	_, ok := transportError(err)
	return ok
}
