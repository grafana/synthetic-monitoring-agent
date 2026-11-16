package logger

import "fmt"

// Redacting returns a Logger that passes each string, []byte, error and fmt.Stringer in the
// keyvals through redact before l logs it. With a nil redact, it returns l.
func Redacting(l Logger, redact func(string) string) Logger {
	if redact == nil {
		return l
	}

	return redactingLogger{logger: l, redact: redact}
}

type redactingLogger struct {
	logger Logger
	redact func(string) string
}

// unredactedKeys hold values that cannot carry a secret. The scraper parses ts and time and drops
// a line whose timestamp does not parse, which a short secret such as "16" could cause.
var unredactedKeys = map[any]bool{"level": true, "time": true, "ts": true}

func (r redactingLogger) Log(keyvals ...any) error {
	redacted := make([]any, len(keyvals))

	for i, kv := range keyvals {
		// Keys come from code, so only values are redacted.
		if i%2 == 0 || unredactedKeys[keyvals[i-1]] {
			redacted[i] = kv

			continue
		}

		switch v := kv.(type) {
		case string:
			redacted[i] = r.redact(v)
		case []byte:
			redacted[i] = r.redact(string(v))
		case error:
			redacted[i] = r.redactText(kv, v.Error())
		case fmt.Stringer:
			redacted[i] = r.redactText(kv, v.String())
		default:
			redacted[i] = kv
		}
	}

	return r.logger.Log(redacted...)
}

// redactText returns kv as is unless redact changes its text, because a logger further down can
// look for a type, such as a go-kit level.Value.
func (r redactingLogger) redactText(kv any, text string) any {
	if out := r.redact(text); out != text {
		return out
	}

	return kv
}
