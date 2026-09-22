//go:build unix

package k6runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLocalRunStopsProcessTreeOnTimeout checks that a timed out check leaves no
// processes behind. Without Setpgid and cmd.Cancel, exec.CommandContext signals k6
// alone and PID 1 adopts whatever k6 started. The fake acts as k6: it starts a
// grandchild, then waits past the timeout.
func TestLocalRunStopsProcessTreeOnTimeout(t *testing.T) {
	t.Parallel()

	runner, pidFile := newSpawnerRunner(t)

	const checkTimeout = 1000 * time.Millisecond

	script := Script{
		Script: []byte("// not executed by the fake"),
		Settings: Settings{
			Timeout: checkTimeout.Milliseconds(),
		},
	}

	start := time.Now()
	resp, runErr := runner.Run(context.Background(), script, SecretStore{}, "test-execution-id")
	runDuration := time.Since(start)

	t.Logf("Run returned after %s (err=%v)", runDuration, runErr)

	// Proves the timeout path ran, and not a check that simply passed. Run reports a
	// timeout in the RunResponse, not as a Go error, because isUserError accepts it.
	require.NoError(t, runErr)
	require.NotNil(t, resp)
	require.Equal(t, ErrorCodeTimeout, resp.ErrorCode, "check did not hit the timeout path: %q", resp.Error)

	pid := readPIDFile(t, pidFile)
	t.Logf("fake k6 reported grandchild PID %d", pid)

	requireGrandchildStopped(t, pid, "the check timeout")
}

// TestLocalRunStopsProcessTreeOnCancel covers agent shutdown. SIGTERM makes the agent
// cancel the check context, and k6 sits in its own group now, so that context is the
// only thing still reaching it.
func TestLocalRunStopsProcessTreeOnCancel(t *testing.T) {
	t.Parallel()

	runner, pidFile := newSpawnerRunner(t)

	// Long enough that the timeout cannot be what stops the tree.
	script := Script{
		Script: []byte("// not executed by the fake"),
		Settings: Settings{
			Timeout: (30 * time.Second).Milliseconds(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runResult struct {
		resp *RunResponse
		err  error
	}

	done := make(chan runResult, 1)

	go func() {
		resp, err := runner.Run(ctx, script, SecretStore{}, "test-execution-id")
		done <- runResult{resp: resp, err: err}
	}()

	// Cancelling before the grandchild exists would pass for the wrong reason.
	pid := readPIDFile(t, pidFile)
	t.Logf("fake k6 reported grandchild PID %d", pid)

	// This is what the agent does when it gets SIGTERM.
	start := time.Now()

	cancel()

	var result runResult

	select {
	case result = <-done:
		t.Logf("Run returned %s after the cancel (err=%v)",
			time.Since(start).Round(time.Millisecond), result.err)

	case <-time.After(10 * time.Second):
		t.Fatal("Run never returned after the context was cancelled")
	}

	// Proves the cancel ended the run. The code is "aborted" because Go reports a
	// signalled exit as -1, which never reaches the 128 that errorType calls "killed".
	require.NoError(t, result.err)
	require.NotNil(t, result.resp)
	require.Equal(t, ErrorCodeAborted, result.resp.ErrorCode)
	require.Contains(t, result.resp.Error, context.Canceled.Error())

	requireGrandchildStopped(t, pid, "the cancel")
}

// newSpawnerRunner returns a Local runner using testdata/k6-fake-spawner, plus the path
// where the fake writes the grandchild PID. A wrapper carries that path, so these tests
// can use t.Parallel, which t.Setenv would rule out.
func newSpawnerRunner(t *testing.T) (Runner, string) {
	t.Helper()

	fakePath, err := filepath.Abs("testdata/k6-fake-spawner")
	require.NoError(t, err)

	dir := t.TempDir()

	// Outside the runner workdir, which Local.Run deletes before it returns.
	pidFile := filepath.Join(dir, "orphan.pid")

	k6Path := filepath.Join(dir, filepath.Base(fakePath))
	wrapper := "#!/bin/sh\n" +
		"ORPHAN_PIDFILE='" + pidFile + "'\n" +
		"export ORPHAN_PIDFILE\n" +
		"exec '" + fakePath + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(k6Path, []byte(wrapper), 0o755))

	runner, err := New(RunnerOpts{Uri: k6Path})
	require.NoError(t, err)
	require.IsType(t, Local{}, runner)

	return runner, pidFile
}

// requireGrandchildStopped waits for the grandchild to stop running. The reason names
// what should have stopped it, for the failure message.
//
// A zombie counts as stopped, because it has already exited and only a reaping parent
// clears the entry. CI runs the tests in a container whose PID 1 does not reap, so an
// orphan stays a zombie there and kill(pid, 0) keeps succeeding.
func requireGrandchildStopped(t *testing.T, pid int, reason string) {
	t.Helper()

	// So a failed test leaves no sleeping process behind.
	t.Cleanup(func() {
		if err := syscall.Kill(pid, syscall.SIGKILL); err == nil {
			t.Logf("cleanup: grandchild %d was still alive and had to be killed", pid)
		}
	})

	// Removal is not instant, so poll.
	deadline := time.Now().Add(5 * time.Second)

	for {
		// Signal 0 only checks that the process exists.
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return
		}

		ppid, state, command := describeProcess(t, pid)
		if strings.HasPrefix(state, "Z") {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf(
				"grandchild %d is still running after %s (ppid = %q, state = %q, command = %q). "+
					"A ppid of 1 means it lost its parent and was adopted, so it really leaked. "+
					"The command shows this is the process the fake started, and not a reused PID.",
				pid, reason, ppid, state, command,
			)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// readPIDFile waits until the fake writes the grandchild PID, then returns it.
func readPIDFile(t *testing.T, path string) int {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid > 0 {
				return pid
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("fake k6 never wrote a usable PID to %q: %v", path, err)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// describeProcess returns the parent PID, state and command. ppid 1 means the process
// was adopted. State separates a live process from a zombie. Command rules out a PID
// that was reused by something else.
func describeProcess(t *testing.T, pid int) (string, string, string) {
	t.Helper()

	const unknown = "unknown"

	out, err := exec.Command("ps", "-o", "ppid=,stat=,command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return unknown, unknown, unknown
	}

	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return strings.TrimSpace(string(out)), unknown, unknown
	}

	return fields[0], fields[1], strings.Join(fields[2:], " ")
}
