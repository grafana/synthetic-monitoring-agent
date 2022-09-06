package k6

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type runner struct {
	script []byte
}

func newRunner(script []byte) (runner, error) {
	r := runner{
		script: script,
	}

	return r, nil
}

func (r runner) Run(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "*")
	if err != nil {
		return err
	}

	defer os.RemoveAll(dir)

	scriptFn, err := createTempFile("script", dir, "script-*.js", r.script)
	if err != nil {
		return err
	}

	summaryFn, err := createTempFile("summary", dir, "summary-*.json", nil)
	if err != nil {
		return err
	}

	logFn, err := createTempFile("log", dir, "log-*", nil)
	if err != nil {
		return err
	}

	// TODO(mem): figure out a way to run this process, possibly in a
	// sandbox or another container.
	//#nosec see above
	exec.CommandContext(
		ctx,
		"k6",
		"run",
		scriptFn,
		"--summary-export",
		summaryFn,
		"--log-format=logfmt",
		"--log-output=file="+logFn,
		"--no-color",
		"--no-summary",
		"--verbose",
		"--quiet",
	)

	return nil
}

func createTempFile(tag, dir, pattern string, content []byte) (string, error) {
	fh, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("creating temporary file for %s: %w", tag, err)
	}

	if len(content) > 0 {
		_, err = fh.Write(content)
		if err != nil {
			_ = fh.Close()
			return "", fmt.Errorf("writing %s to file %s: %w", tag, fh.Name(), err)
		}
	}

	err = fh.Close()
	if err != nil {
		return "", fmt.Errorf("closing file %s for %s: %w", fh.Name(), tag, err)
	}

	return fh.Name(), nil
}
