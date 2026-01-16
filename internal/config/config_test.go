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
