acp-gate
===

A small, transparent ACP proxy that sits between your editor (upstream) and a real ACP agent (downstream). It forwards all ACP traffic and writes an audit trail to a local SQLite database.

Built on top of github.com/coder/acp-go-sdk.

Features
-
- Transparent stdin/stdout proxy between editor and agent
- Persistent audit log (SQLite) of all requests/responses
- Best‑effort extraction of user/agent text for easier browsing
- Simple JSON configuration with named agent servers
- CLI overrides for command, args, and env merging

Requirements
-
- Go 1.25+

Install
-
```
# From repository root
go build -o acp-gate
# Or install into your GOPATH/bin
go install ./...
```

Usage
-
acp-gate is intended to be launched by your editor/IDE in place of the real agent. It will then spawn the real agent and proxy all messages while auditing them.

Command-line flags:
- -audit-db string
  Path to SQLite audit DB (default: audit.sqlite)
- -config string
  Path to JSON config file with agent_servers (default: auto-detected; see below)
- -agent-name string
  Agent server name from config to use
- -agent-cmd string
  Downstream real agent command (overrides config)
- -agent-arg value
  Argument for downstream agent (repeatable)

Examples:

1) Run with explicit downstream command
```
./acp-gate \
  -agent-cmd /usr/local/bin/my-acp-agent \
  -agent-arg --model \
  -agent-arg gpt-4o
```

2) Run using a config-defined agent
```
./acp-gate -config ~/.config/acp-gate/config.json -agent-name openai
```

Configuration
-
acp-gate looks for a config file when -config is not provided. Order of preference:
1) $XDG_CONFIG_HOME/.acp-gate/config.json
2) $XDG_CONFIG_HOME/acp-gate/config.json
3) ~/.config/.acp-gate/config.json
4) ~/.config/acp-gate/config.json

File format (JSON):
```json
{
  "agent_servers": {
    "openai": {
      "command": "~/bin/acp-agent-openai",
      "args": ["--model", "gpt-4o"],
      "env": {
        "OPENAI_API_KEY": "..."
      }
    }
  }
}
```

Notes:
- -agent-cmd overrides the configured command.
- Final argv is config.args followed by any -agent-arg flags.
- Environment variables from config.env are merged into the base process env.
- A leading ~ in the command path is expanded to the current user’s home directory.

Auditing
-
By default acp-gate writes to ./audit.sqlite. Each record contains:
- Timestamp
- Direction (upstream_to_downstream or downstream_to_upstream)
- Session ID (when available)
- Method name
- Raw JSON payload
- Best‑effort extracted user_text/agent_text (for prompt/response chunks and updates)

Be mindful of sensitive data: the audit DB can include full prompt and response contents.

Development
-
Run tests:
```
go test ./...
```

License
-
This project is licensed under the terms of the LICENSE file in this repository.