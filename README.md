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
- Handy CLI overrides for command, args, and env

Download & Install
-
1) Download the latest release for your OS/arch from:
   https://github.com/Tryanks/acp-gate/releases/latest

2) Unpack the archive and place the acp-gate binary somewhere on your PATH
   (for example: /usr/local/bin on macOS/Linux, or add the folder to PATH on Windows).

3) macOS/Linux users may need to make it executable:
   chmod +x acp-gate

Quick start
-
acp-gate is intended to be launched by your editor/IDE in place of the real agent. It will then spawn the real agent and proxy all messages while auditing them.

Examples:

1) Run with an explicit downstream command
```
acp-gate \
  -agent-cmd /usr/local/bin/my-acp-agent \
  -agent-arg --model \
  -agent-arg gpt-4o
```

2) Run using a config-defined agent
```
acp-gate -config ~/.config/acp-gate/config.json -agent-name openai
```

Command-line flags (common)
-
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

Configuration
-
If -config is not provided, acp-gate looks for a config file in this order:
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

Privacy note: the audit DB can include full prompt and response contents. Handle it with care.

License
-
This project is licensed under the terms of the LICENSE file in this repository.