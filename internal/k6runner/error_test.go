package k6runner

import (
	"context"
	"errors"
	"os/exec"
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
			// Captured from k6 running a browser script with a browser that does not exist.
			name:     "browser not found",
			logLines: `time="2026-11-11T16:21:21-05:00" level=error msg="error building browser on IterStart: finding browser executable: k6 couldn't detect google chrome or a chromium-supported browser on the given path: /nonexistent/chromium at default (file:///tmp/script.js:12:16(0))"`,
			expect:   ErrBrowserLaunch,
		},
		{
			// Captured from k6 running a browser script with a browser that exits right away.
			name: "browser exits right away",
			logLines: `
time="2026-11-11T16:21:32-05:00" level=error msg="process with PID 954076 unexpectedly ended: exit status 1" category=browser elapsed="0 ms" source=browser
time="2026-11-11T16:21:32-05:00" level=error msg="error building browser on IterStart: launching browser: browser process ended unexpectedly at default (file:///tmp/script.js:12:16(0))"
			`,
			expect: ErrBrowserLaunch,
		},
		{
			// The script can print anything, including this. What a script prints has source=console.
			name:     "script prints the message of a launch failure",
			logLines: `time="2026-11-11T16:24:26-05:00" level=error msg="error building browser on IterStart: launching browser: fake" source=console`,
			expect:   nil,
		},
		{
			name:     "same words, not an error",
			logLines: `level=info msg="error building browser on IterStart: launching browser: fake"`,
			expect:   nil,
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

func TestErrorType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		expect string
	}{
		{name: "no error", err: nil, expect: ErrorCodeNone},
		{name: "stack trace", err: ErrStacktrace, expect: ErrorCodeAborted},
		{name: "thrown", err: ErrThrown, expect: ErrorCodeFailed},
		{name: "something else", err: errors.New("who knows"), expect: ErrorCodeUnknown},
		{name: "browser launch", err: ErrBrowserLaunch, expect: ErrorCodeBrowserLaunch},
		{name: "browser launch and an exit code", err: errors.Join(&exec.ExitError{}, ErrBrowserLaunch), expect: ErrorCodeBrowserLaunch},
		// Running out of time is what happened, even if the browser was slow to launch.
		{name: "timeout while launching", err: errors.Join(context.DeadlineExceeded, ErrBrowserLaunch), expect: ErrorCodeTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := errorType(tc.err); got != tc.expect {
				t.Fatalf("errorType() = %q, expected %q", got, tc.expect)
			}
		})
	}
}
