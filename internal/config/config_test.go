package config

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestLoadAndSelect(t *testing.T) {
    json := `{
        "agent_servers": {
            "Codex Cli": {
                "command": "~/Documents/codex-acp",
                "args": ["--foo"],
                "env": {"CHATANYWHERE_API_KEY": "sk-abc"}
            },
            "OpenCode": {
                "command": "/bin/true",
                "args": ["acp"]
            }
        }
    }`

    dir := t.TempDir()
    p := filepath.Join(dir, "cfg.json")
    if err := os.WriteFile(p, []byte(json), 0o644); err != nil {
        t.Fatalf("write temp cfg: %v", err)
    }

    cfg, err := Load(p)
    if err != nil {
        t.Fatalf("Load: %v", err)
    }

    as, ok := SelectAgent(cfg, "Codex Cli")
    if !ok {
        t.Fatalf("SelectAgent failed")
    }
    if len(as.Args) != 1 || as.Args[0] != "--foo" {
        t.Fatalf("unexpected args: %+v", as.Args)
    }
}

func TestExpandUser(t *testing.T) {
    home, _ := os.UserHomeDir()
    got := ExpandUser("~/x/y")
    if home != "" && !strings.HasPrefix(got, home+string(os.PathSeparator)) {
        t.Fatalf("ExpandUser did not prefix home: %q not in %q", got, home)
    }
}

func TestMergeEnv(t *testing.T) {
    base := []string{"A=1", "PATH=/bin"}
    overlay := map[string]string{"A": "2", "NEW": "3"}
    out := MergeEnv(base, overlay)
    // Turn back to map for assertions
    m := map[string]string{}
    for _, kv := range out {
        i := strings.IndexByte(kv, '=')
        if i > 0 {
            m[kv[:i]] = kv[i+1:]
        }
    }
    if m["A"] != "2" || m["NEW"] != "3" || m["PATH"] != "/bin" {
        t.Fatalf("unexpected env: %+v", m)
    }
}

func TestResolve(t *testing.T) {
    cfg := Config{AgentServers: map[string]AgentServer{
        "OpenCode": {Command: "/bin/echo", Args: []string{"acp"}, Env: map[string]string{"X": "1"}},
    }}
    cmd, args, env, err := Resolve(cfg, "OpenCode", "", []string{"--debug"}, []string{"Y=2"})
    if err != nil {
        t.Fatalf("Resolve error: %v", err)
    }
    if !strings.HasSuffix(cmd, "/bin/echo") {
        t.Fatalf("unexpected cmd: %q", cmd)
    }
    if len(args) != 2 || args[0] != "acp" || args[1] != "--debug" {
        t.Fatalf("unexpected args: %+v", args)
    }
    mv := map[string]string{}
    for _, kv := range env {
        i := strings.IndexByte(kv, '=')
        if i > 0 {
            mv[kv[:i]] = kv[i+1:]
        }
    }
    if mv["X"] != "1" || mv["Y"] != "2" {
        t.Fatalf("unexpected env: %+v", mv)
    }
}

func TestDefaultConfigPaths_XDG(t *testing.T) {
    t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
    paths := DefaultConfigPaths()
    if len(paths) < 2 {
        t.Fatalf("expected at least 2 paths, got %d", len(paths))
    }
    if !strings.HasSuffix(paths[0], filepath.Join(".acp-gate", "config.json")) {
        t.Fatalf("unexpected first default path: %q", paths[0])
    }
    if !strings.Contains(paths[0], string(os.PathSeparator)+"xdg"+string(os.PathSeparator)) {
        t.Fatalf("first path should use XDG_CONFIG_HOME, got %q", paths[0])
    }
}

func TestFindExistingDefaultConfig(t *testing.T) {
    // Force lookup under HOME .config
    home := t.TempDir()
    t.Setenv("XDG_CONFIG_HOME", "")
    t.Setenv("HOME", home)
    // Create ~/.config/.acp-gate/config.json
    p := filepath.Join(home, ".config", ".acp-gate", "config.json")
    if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
        t.Fatalf("mkdir: %v", err)
    }
    if err := os.WriteFile(p, []byte(`{"agent_servers":{}}`), 0o644); err != nil {
        t.Fatalf("write: %v", err)
    }
    got, ok := FindExistingDefaultConfig()
    if !ok {
        t.Fatalf("expected to find default config")
    }
    if got != p {
        t.Fatalf("expected %q, got %q", p, got)
    }
}
