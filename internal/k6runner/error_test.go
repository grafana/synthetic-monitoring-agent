package k6runner

import (
	"errors"
	"testing"
)

func TestErrorFromLogs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		logLines string
		expect   error
	}{
		{
			name:     "console.error",
			logLines: `time="2024-04-18T19:39:22+02:00" level=error msg="console error" source=console`,
			expect:   nil,
		},
		{
			name: "stack trace",
			logLines: `
			time="2024-04-18T19:39:22+02:00" level=error msg="console error" source=console
			level=error msg=foobar source=stacktrace
			`,
			expect: ErrStacktrace,
		},
		{
			name: "unsupported browser",
			logLines: `
time="2024-07-12T16:47:13+02:00" level=error msg="Uncaught (in promise) GoError: browser not found in registry. make sure to set browser type option in scenario definition in order to use the browser module\n\tat github.com/grafana/xk6-browser/browser.syncMapBrowser.func7 (native)\n\tat file:///tmp/roobre/browser.js:20:15(4)\n" executor=shared-iterations scenario=default
			`,
			expect: ErrThrown,
		},
		{
			name: "throw new Error()",
			logLines: `
time="2024-07-12T16:48:01+02:00" level=error msg="Uncaught (in promise) Error: foobar\n\tat file:///tmp/roobre/browser.js:22:8(8)\n" executor=shared-iterations scenario=ui
			`,
			expect: ErrThrown,
		},
		{
			name: "required filds in different lines",
			logLines: `
			lever=error msg=something
			source=stacktrace msg=something
			`,
			expect: nil,
		},
		{
			name: "badly formatted lines and stactrace",
			//nolint:dupword // Duped
			logLines: `
			something something
			lever=error something something
			source=stacktrace something something
			level=error msg="missing a quote
			level=error msg=foobar source=stacktrace
			`,
			expect: nil, // FIXME: Probably the parser should tolerate malformed lines.
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := errorFromLogs([]byte(tc.logLines))
			if !errors.Is(err, tc.expect) {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}
