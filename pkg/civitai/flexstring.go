package civitai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// FlexString is a JSON field the Civitai API documents as a string but does not
// always SEND as one. AGENTS.md item 37; the full rationale, the float64
// comparison table and the enumerated non-decisions are in
// claudedocs/decisions/37-numeric-username.md.
//
// 🔴 IT IS NOT A `string`, AND "FIXING" IT BACK TO ONE RE-BREAKS `images search`.
// A Civitai username may be entirely digits (there are real accounts like
// https://civitai.com/user/2802169344506), and such a value was REPORTED
// arriving as a BARE JSON NUMBER — `"username": 2802169344506`, unquoted.
// `encoding/json` refuses a number into a `string` field, so the WHOLE page
// decode failed and `civitai images search` exited 1 with the opaque
// "unexpected response from /api/v1/images (status 200)". Reported by Rochet2
// in civitai/cli#513.
//
// 🔴 CONFIRMED LIVE 2026-09-11. An earlier note here said the shape was NOT
// reproducible and that /api/v1/images "could not be exercised". Both were
// wrong. /api/v1/images DOES coerce, on every request reaching the Meilisearch
// feed branch of runImageSearch: 51/51 cache-busted responses came back
// unquoted. The 2026-09-09 reading that said otherwise was measuring Cloudflare
// cache HITs, not the origin — vary a query param and require
// cf-cache-status: MISS before believing a negative here. (/api/v1/users really
// is quoted; it is Prisma-backed, a different path.) The original justification
// still stands on its own: a client must not hard-fail a 200 whose body it can
// read. Tracked server-side as civitai/civitai#4768.
//
// 🔴 THIS TYPE MAKES THE VALUE DECODABLE, NOT CORRECT. The coercion is NUMERIC,
// so leading zeros are destroyed upstream: a user really named "0222" reaches us
// as 222, and 222 matches no account (?username=222 returns 0 items). FlexString
// stops the page hard-failing, which is its whole job; it cannot recover digits
// the wire already dropped. Do not read "#513 is fixed" as "the username printed
// is the username that exists" — that awaits the server fix. See the decision doc.
//
// The coercion is deliberately NARROW — a JSON string and a JSON number are the
// two shapes at issue, plus `null`, which every one of these fields was already
// nullable for. A boolean, an array and an object are still hard errors: this
// type exists to absorb ONE reported server shape, not to hide schema drift
// behind a lenient decoder.
//
// 🔴 A NUMBER KEEPS ITS LITERAL DIGITS, NEVER A `float64` ROUND TRIP. Decoding
// 2802169344506 through `float64` and re-rendering it yields "2.802169344506e+12"
// — a username that matches nothing and that a user cannot paste back into
// `--username`. The number branch goes through json.Number, which hands back the
// source bytes unchanged, so a 13-digit id survives byte-for-byte and a float
// literal stays exactly as the server wrote it ("1.0" stays "1.0"). Pinned by
// TestFlexStringPreservesBigIntegerDigits.
//
// It is a defined string type, so comparisons against string constants
// (`u.Username != ""`) still compile at every call site; passing one to a
// `func(string)` needs an explicit conversion or .String().
//
// MARSHALLING: FlexString marshals with the default string rules, i.e. a
// numeric username would be re-emitted QUOTED. That is not a live output-shape
// change: nothing in this repo marshals the structs these fields sit on, and the
// `--json` contract is served from the RAW server bytes (internal/cmd/read.go's
// emitJSON), never from a re-encode. Adding a MarshalJSON here would therefore
// only change an output nobody produces — leave it alone until something does.
type FlexString string

// String returns the decoded value as a plain Go string.
func (f FlexString) String() string { return string(f) }

// UnmarshalJSON accepts a JSON string or a JSON number, and treats null as
// "field absent" (a no-op leaving the zero value), which is what a plain
// `string` field did before this type existed — several of these fields are
// documented as nullable server-side.
func (f *FlexString) UnmarshalJSON(b []byte) error {
	raw := bytes.TrimSpace(b)
	if len(raw) == 0 {
		return errors.New("cannot decode an empty JSON value into a string field")
	}
	switch raw[0] {
	case 'n':
		if string(raw) == "null" {
			// Leave the zero value, matching encoding/json's own null handling
			// for a string field.
			return nil
		}
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		// json.Number is the whole point: it carries the LITERAL source digits,
		// so a big integer id is not laundered through float64.
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return err
		}
		*f = FlexString(n.String())
		return nil
	}
	return fmt.Errorf("cannot decode JSON %s into a string field: want a JSON string or number", jsonKind(raw))
}

// jsonKind names the JSON value kind of raw for an error message, from its first
// byte alone. It is only ever called on a value UnmarshalJSON above has already
// declined to handle, so it never has to name a string or a number.
func jsonKind(raw []byte) string {
	switch raw[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	default:
		return fmt.Sprintf("value %s", snippet(raw))
	}
}
