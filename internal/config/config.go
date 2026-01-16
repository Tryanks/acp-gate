package config

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// AgentServer describes one agent executable with optional args and env.
type AgentServer struct {
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env"`
}

// Config is the root configuration file structure.
type Config struct {
    AgentServers map[string]AgentServer `json:"agent_servers"`
}

// Load reads and parses a configuration from a JSON file.
func Load(path string) (Config, error) {
    var cfg Config
    b, err := os.ReadFile(path)
    if err != nil {
        return cfg, err
    }
    if err := json.Unmarshal(b, &cfg); err != nil {
        return cfg, err
    }
    return cfg, nil
}

// SelectAgent returns the AgentServer configuration for a given name.
func SelectAgent(cfg Config, name string) (AgentServer, bool) {
    if cfg.AgentServers == nil {
        return AgentServer{}, false
    }
    as, ok := cfg.AgentServers[name]
    return as, ok
}

// OnlyAgent returns the single configured agent if there is exactly one.
func OnlyAgent(cfg Config) (string, AgentServer, bool) {
    if len(cfg.AgentServers) != 1 {
        return "", AgentServer{}, false
    }
    for k, v := range cfg.AgentServers {
        return k, v, true
    }
    return "", AgentServer{}, false
}

// ExpandUser performs a basic expansion of a leading ~ to the user's home dir.
func ExpandUser(path string) string {
    if path == "" {
        return path
    }
    if path[0] != '~' {
        return path
    }
    home, err := os.UserHomeDir()
    if err != nil || home == "" {
        return path
    }
    if path == "~" {
        return home
    }
    // handle ~user not supported; keep as-is
    if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
        return path
    }
    return filepath.Join(home, path[2:])
}

// MergeEnv overlays the provided kv map onto a base environment slice.
// Keys are case-sensitive as in the host OS.
func MergeEnv(base []string, overlay map[string]string) []string {
    if overlay == nil || len(overlay) == 0 {
        return append([]string(nil), base...)
    }
    // turn base into a map
    m := make(map[string]string, len(base)+len(overlay))
    order := make([]string, 0, len(base))
    for _, kv := range base {
        if kv == "" {
            continue
        }
        eq := strings.IndexByte(kv, '=')
        if eq <= 0 {
            continue
        }
        k := kv[:eq]
        v := kv[eq+1:]
        if _, seen := m[k]; !seen {
            order = append(order, k)
        }
        m[k] = v
    }
    // apply overlay
    for k, v := range overlay {
        if _, seen := m[k]; !seen {
            order = append(order, k)
        }
        m[k] = v
    }
    // rebuild slice
    out := make([]string, 0, len(m))
    for _, k := range order {
        out = append(out, fmt.Sprintf("%s=%s", k, m[k]))
    }
    // include potential new keys not in order (map iteration order) — should not happen
    if len(out) != len(m) {
        for k, v := range m {
            found := false
            for _, ok := range order {
                if ok == k {
                    found = true
                    break
                }
            }
            if !found {
                out = append(out, fmt.Sprintf("%s=%s", k, v))
            }
        }
    }
    return out
}

// Resolve determines the final command, args, and env according to config and cli overrides.
// If cliCmd is non-empty, it overrides the configured command.
// The resulting args are config.Args followed by cliArgs.
func Resolve(cfg Config, agentName, cliCmd string, cliArgs []string, baseEnv []string) (cmd string, args []string, env []string, err error) {
    var as AgentServer
    var ok bool
    if agentName != "" {
        as, ok = SelectAgent(cfg, agentName)
        if !ok {
            return "", nil, nil, fmt.Errorf("agent %q not found in config", agentName)
        }
    } else if name, one, hasOne := OnlyAgent(cfg); hasOne {
        _ = name
        as = one
    }

    // Determine command
    cmd = cliCmd
    if cmd == "" {
        cmd = as.Command
    }
    cmd = ExpandUser(cmd)
    if cmd == "" {
        return "", nil, nil, errors.New("no agent command provided (use -agent-cmd or config)")
    }

    // Determine args
    args = append([]string{}, as.Args...)
    if len(cliArgs) > 0 {
        args = append(args, cliArgs...)
    }

    // Determine env
    env = MergeEnv(baseEnv, as.Env)
    return cmd, args, env, nil
}

// DefaultConfigPaths returns preferred config file locations to check
// when the -config flag is not provided.
// Order of preference:
//   1) $XDG_CONFIG_HOME/.acp-gate/config.json
//   2) $XDG_CONFIG_HOME/acp-gate/config.json
//   3) ~/.config/.acp-gate/config.json
//   4) ~/.config/acp-gate/config.json
func DefaultConfigPaths() []string {
    var paths []string

    xdg := os.Getenv("XDG_CONFIG_HOME")
    if xdg != "" {
        paths = append(paths,
            filepath.Join(xdg, ".acp-gate", "config.json"),
            filepath.Join(xdg, "acp-gate", "config.json"),
        )
    }

    if home, err := os.UserHomeDir(); err == nil && home != "" {
        base := filepath.Join(home, ".config")
        paths = append(paths,
            filepath.Join(base, ".acp-gate", "config.json"),
            filepath.Join(base, "acp-gate", "config.json"),
        )
    }

    return paths
}

// FindExistingDefaultConfig returns the first existing config file among
// DefaultConfigPaths. If none exist, ok is false.
func FindExistingDefaultConfig() (path string, ok bool) {
    for _, p := range DefaultConfigPaths() {
        if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
            return p, true
        }
    }
    return "", false
}
