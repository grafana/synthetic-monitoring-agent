package version

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog"
)

// Repository points to a collection of k6 binaries.
type Repository struct {
	// Root points to a folder in the local filesystem that will be scanned for k6 binaries.
	// All executable files in said folder should be k6 binaries, as they will all be executed with `--version` to map
	// their actual versions.
	Root string
	// Override is the path to a specific k6 binary. If set, all Repository operations will return this path.
	Override string

	// Logger.
	Logger zerolog.Logger

	mtx     sync.Mutex
	entries []Entry
}

const binaryMustContain = "k6"

type Entry struct {
	Path    string
	Version *semver.Version
}

type k6Version struct {
	Commit    string `json:"commit"`
	GoArch    string `json:"go_arch"`
	GoOs      string `json:"go_os"`
	GoVersion string `json:"go_version"`
	Version   string `json:"version"`
}

func (r *Repository) Entries() ([]Entry, error) {
	err := r.scan(false)
	if err != nil {
		return nil, fmt.Errorf("scanning for binaries: %w", err)
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	if len(r.entries) == 0 {
		return nil, nil
	}

	entries := make([]Entry, len(r.entries))
	copy(entries, r.entries)

	return entries, nil
}

func (r *Repository) scan(force bool) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if len(r.entries) > 0 && !force {
		return nil
	}

	// All binaries found in the repository folder, which must also match a naming pattern, are assumed to be indeed k6
	// binaries. The code will try to execute them with `$0 version --json` and error out entirely if any of them
	// misbehave.
	binaries, err := r.binaries()
	if err != nil {
		return err
	}

	for _, bin := range binaries {
		k6Version, err := runK6Version(bin)
		if err != nil {
			return fmt.Errorf("finding version for %q: %w", bin, err)
		}

		version, err := semver.NewVersion(k6Version.Version)
		if err != nil {
			return fmt.Errorf("version %q returned by %q is invalid: %w", k6Version.Version, bin, err)
		}

		r.entries = append(r.entries, Entry{
			Path:    bin,
			Version: version,
		})
	}

	return nil
}

func (r *Repository) binaries() ([]string, error) {
	if r.Override != "" {
		r.Logger.Warn().Str("k6", r.Override).Msg("Overriding k6 binary autoselection")

		return []string{r.Override}, nil
	}

	var binaries []string

	files, err := fs.ReadDir(os.DirFS(r.Root), ".")
	if err != nil {
		return nil, fmt.Errorf("reading k6 repository root: %w", err)
	}

	for _, file := range files {
		path := filepath.Join(r.Root, file.Name())

		if file.IsDir() {
			r.Logger.Warn().Str("root", r.Root).Str("directory", path).Msg("Foreign directory found inside k6 repository root")
			continue
		}

		info, err := file.Info()
		if err != nil {
			return nil, fmt.Errorf("reading file info: %w", err)
		}

		if info.Mode().Perm()&0o111 == 0 {
			// This is not an exhaustive check: It is possible that the file is executable, but not by the user running
			// this code, in which case the error will be thrown later. This is a best-effort pass to detect stray
			// files.
			r.Logger.Warn().Str("root", r.Root).Str("path", path).Msg("Found non-executable file inside k6 repository root")
			continue
		}

		if !strings.Contains(file.Name(), binaryMustContain) {
			// Ignore binaries that do not contain a specific substring in the name. In the next step we will execute
			// every found binary with `--version --json`, so as a safety check to avoid executing unknown binaries if
			// we're pointed to the wrong directory (like /usr/bin) we look for a specific name here.
			r.Logger.Warn().Str("root", r.Root).Str("path", path).Msg("Foreign binary found inside k6 repository root")
			continue
		}

		binaries = append(binaries, path)
	}

	return binaries, nil
}

func runK6Version(k6Path string) (*k6Version, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, k6Path, "--version", "--json")
	cmd.Env = []string{
		"K6_AUTO_EXTENSION_RESOLUTION=false",
		// By not explicitly appending os.Env, all other env vars are cleared here.
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("running k6: %w\n%s", err, stderr.String())
	}

	k6v := k6Version{}
	err = json.Unmarshal(stdout.Bytes(), &k6v)
	if err != nil {
		return nil, fmt.Errorf("parsing json: %w", err)
	}

	return &k6v, nil
}
