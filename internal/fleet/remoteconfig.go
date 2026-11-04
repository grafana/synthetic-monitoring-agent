package fleet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

const (
	// remoteConfigKey is the top-level YAML key the agent reads from the
	// configuration that Fleet Management sends. It is also accepted under
	// "extensions", in case Fleet Management requires OTel-shaped pipelines.
	remoteConfigKey      = "synthetic_monitoring"
	remoteConfigDumpFile = "fm-remote-config.txt"
	effectiveConfigName  = "synthetic_monitoring.yaml"
)

var errDuplicateRemoteConfig = errors.New("synthetic_monitoring is defined in more than one pipeline")

// remoteSettings are the settings Fleet Management may change. Anything not
// listed here is rejected.
type remoteSettings struct {
	LogLevel    string `yaml:"log_level,omitempty"`
	K6HTTPDebug string `yaml:"k6_http_debug,omitempty"`
}

// K6HTTPDebug is the k6 HTTP debug mode the agent applies to k6 runs.
type K6HTTPDebug interface {
	Set(mode string) error
	Get() string
}

// handleRemoteConfig validates and applies the configuration offered by Fleet
// Management, and reports the outcome back to it.
func (a *Agent) handleRemoteConfig(ctx context.Context, c client.OpAMPClient, rc *protobufs.AgentRemoteConfig) {
	status := a.processRemoteConfig(rc)
	if status == nil {
		return
	}

	a.mu.Lock()
	if err := c.SetAgentDescription(a.description()); err != nil {
		a.logger.Warn().Err(err).Msg("failed to update fleet management agent description")
	}
	a.mu.Unlock()

	// rcMu must not be held here: UpdateEffectiveConfig calls back into
	// GetEffectiveConfig synchronously, which takes it.
	if err := c.SetRemoteConfigStatus(status); err != nil {
		a.logger.Warn().Err(err).Msg("failed to report remote config status to fleet management")
	}

	if err := c.UpdateEffectiveConfig(ctx); err != nil {
		a.logger.Warn().Err(err).Msg("failed to report effective config to fleet management")
	}
}

// processRemoteConfig applies rc and returns the status to report, or nil if
// rc was already processed.
func (a *Agent) processRemoteConfig(rc *protobufs.AgentRemoteConfig) *protobufs.RemoteConfigStatus {
	a.rcMu.Lock()
	defer a.rcMu.Unlock()

	if a.rcHash != nil && string(a.rcHash) == string(rc.GetConfigHash()) {
		return nil
	}

	a.rcHash = rc.GetConfigHash()

	a.logRemoteConfig(rc)

	status := &protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: rc.GetConfigHash(),
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
	}

	if err := a.applyRemoteConfig(rc.GetConfig()); err != nil {
		a.logger.Warn().Err(err).Msg("rejected remote config from fleet management")
		status.Status = protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED
		status.ErrorMessage = err.Error()
	}

	return status
}

// applyRemoteConfig must be called with a.rcMu held.
func (a *Agent) applyRemoteConfig(cfg *protobufs.AgentConfigMap) error {
	settings, found, err := parseRemoteConfig(cfg)
	if err != nil {
		return err
	}

	// Validate everything before changing anything, so that a rejected
	// config leaves the agent as it was.
	level := a.defaultLevel
	if settings.LogLevel != "" {
		level, err = parseLogLevel(settings.LogLevel)
		if err != nil {
			return err
		}
	}

	httpDebug := settings.K6HTTPDebug
	if httpDebug == "" {
		httpDebug = "off"
	}

	switch {
	case httpDebug != "off" && httpDebug != "headers" && httpDebug != "full":
		return fmt.Errorf("invalid k6_http_debug %q: must be off, headers or full", settings.K6HTTPDebug)

	case httpDebug != "off" && a.k6HTTPDebug == nil:
		return errors.New("k6_http_debug requires k6, which is not enabled on this probe")
	}

	if a.k6HTTPDebug != nil {
		if err := a.k6HTTPDebug.Set(httpDebug); err != nil {
			return err
		}
	}

	zerolog.SetGlobalLevel(level)
	a.applied = settings

	a.logger.WithLevel(zerolog.InfoLevel).
		Bool("found", found).
		Str("log_level", level.String()).
		Str("k6_http_debug", httpDebug).
		Msg("applied remote config from fleet management")

	return nil
}

// effectiveConfig must be called with a.rcMu held.
func (a *Agent) effectiveConfig() (*protobufs.EffectiveConfig, error) {
	effective := a.applied
	effective.LogLevel = zerolog.GlobalLevel().String()
	effective.K6HTTPDebug = ""

	if a.k6HTTPDebug != nil {
		effective.K6HTTPDebug = a.k6HTTPDebug.Get()
	}

	body, err := yaml.Marshal(map[string]remoteSettings{remoteConfigKey: effective})
	if err != nil {
		return nil, err
	}

	return &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigObject{
				effectiveConfigName: {Body: body, ContentType: "text/yaml"},
			},
		},
	}, nil
}

// logRemoteConfig records the shape of the configuration Fleet Management sent:
// file names, content types and sizes in the log, and the full content in the
// state directory (it may contain secrets, so it's never logged).
func (a *Agent) logRemoteConfig(rc *protobufs.AgentRemoteConfig) {
	names := sortedConfigNames(rc.GetConfig())

	files := zerolog.Arr()
	var dump strings.Builder

	for _, name := range names {
		f := rc.GetConfig().GetConfigMap()[name]
		files.Dict(zerolog.Dict().Str("name", name).Str("content_type", f.GetContentType()).Int("size", len(f.GetBody())))
		fmt.Fprintf(&dump, "### %q (content type %q)\n%s\n", name, f.GetContentType(), f.GetBody())
	}

	a.logger.WithLevel(zerolog.InfoLevel).
		Str("hash", hex.EncodeToString(rc.GetConfigHash())).
		Array("files", files).
		Msg("received remote config from fleet management")

	if a.stateDir == "" {
		return
	}

	if err := os.WriteFile(filepath.Join(a.stateDir, remoteConfigDumpFile), []byte(dump.String()), 0o600); err != nil {
		a.logger.Warn().Err(err).Msg("failed to save remote config")
	}
}

// parseRemoteConfig looks for the synthetic_monitoring block in every file of
// the configuration. It reports whether the block was found, so that an empty
// or unrelated configuration reverts to the defaults.
func parseRemoteConfig(cfg *protobufs.AgentConfigMap) (remoteSettings, bool, error) {
	var (
		settings remoteSettings
		found    bool
	)

	for _, name := range sortedConfigNames(cfg) {
		body := cfg.GetConfigMap()[name].GetBody()
		if len(strings.TrimSpace(string(body))) == 0 {
			continue
		}

		var doc map[string]yaml.Node
		if err := yaml.Unmarshal(body, &doc); err != nil {
			return remoteSettings{}, false, fmt.Errorf("parsing %q: %w", name, err)
		}

		nodes := findRemoteConfigNodes(doc)
		if len(nodes) == 0 {
			continue
		}

		if found || len(nodes) > 1 {
			return remoteSettings{}, false, errDuplicateRemoteConfig
		}

		found = true

		var err error
		if settings, err = decodeRemoteSettings(nodes[0]); err != nil {
			return remoteSettings{}, false, fmt.Errorf("parsing %q: %w", name, err)
		}
	}

	return settings, found, nil
}

// findRemoteConfigNodes returns the synthetic_monitoring blocks in doc, at the
// top level or under "extensions". Like OTel component IDs, the key may carry
// a name: "synthetic_monitoring/probe".
func findRemoteConfigNodes(doc map[string]yaml.Node) []yaml.Node {
	nodes := matchRemoteConfigKeys(doc)

	if ext, ok := doc["extensions"]; ok {
		var extensions map[string]yaml.Node
		if err := ext.Decode(&extensions); err == nil {
			nodes = append(nodes, matchRemoteConfigKeys(extensions)...)
		}
	}

	return nodes
}

func matchRemoteConfigKeys(m map[string]yaml.Node) []yaml.Node {
	var nodes []yaml.Node

	for key, node := range m {
		if key == remoteConfigKey || strings.HasPrefix(key, remoteConfigKey+"/") {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

func decodeRemoteSettings(node yaml.Node) (remoteSettings, error) {
	var raw map[string]any
	if err := node.Decode(&raw); err != nil {
		return remoteSettings{}, err
	}

	var settings remoteSettings

	for key, value := range raw {
		switch key {
		case "log_level":
			s, ok := value.(string)
			if !ok {
				return remoteSettings{}, fmt.Errorf("log_level must be a string, got %v", value)
			}

			settings.LogLevel = s

		case "k6_http_debug":
			s, ok := value.(string)
			if !ok {
				return remoteSettings{}, fmt.Errorf("k6_http_debug must be a string, got %v", value)
			}

			settings.K6HTTPDebug = s

		default:
			return remoteSettings{}, fmt.Errorf("unsupported setting %q", key)
		}
	}

	return settings, nil
}

func parseLogLevel(s string) (zerolog.Level, error) {
	switch level, err := zerolog.ParseLevel(strings.ToLower(s)); {
	case err != nil:
		return zerolog.NoLevel, fmt.Errorf("invalid log_level %q: %w", s, err)

	case level < zerolog.DebugLevel || level > zerolog.ErrorLevel:
		return zerolog.NoLevel, fmt.Errorf("invalid log_level %q: must be debug, info, warn or error", s)

	default:
		return level, nil
	}
}

func sortedConfigNames(cfg *protobufs.AgentConfigMap) []string {
	names := make([]string, 0, len(cfg.GetConfigMap()))
	for name := range cfg.GetConfigMap() {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
