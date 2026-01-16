acp-gate
===

A small, transparent ACP proxy that sits between your editor (upstream) and a real ACP agent (downstream). It forwards all ACP traffic and writes an audit trail to a local SQLite database.

Built on top of github.com/coder/acp-go-sdk.

[中文版](README_zh.md)

Features
-
- Transparent stdin/stdout proxy between editor and agent
- Persistent audit log (SQLite) of all requests/responses
- Best‑effort extraction of user/agent text for easier browsing
- Simple JSON configuration with named agent servers
- Handy CLI overrides for command, args, and env
- Remote mode over gRPC (server/client) to decouple editor and agent host
- Pure proxy server mode to relay bytes to another acp-gate server (no local agent, no auditing)
- Optional proxy hops between client and server (no auditing on hops)
- Server‑side auditing only in remote mode (client/proxy hops do not audit)

Architecture
-
The diagram below shows typical topologies. Only the end server that spawns the real downstream ACP agent performs auditing and writes to SQLite.

```mermaid
flowchart LR
  subgraph editor[Editor Host]
    E["Editor / IDE"]
    C["acp-gate (client)<br/>- stdio bridge<br/>- no auditing"]
  end

  subgraph proxy[Proxy Host (optional)]
    P["acp-gate (pure proxy server)<br/>- relay only<br/>- no auditing"]
  end

  subgraph agent[Agent Host]
    S["acp-gate (server)<br/>- launches downstream agent<br/>- performs auditing"]
    D[Downstream ACP Agent]
    A[(SQLite audit.sqlite)]
  end

  E <-- stdio --> C
  C <-- gRPC tunnel --> P
  P <-- gRPC tunnel --> S
  S <-- stdio --> D
  S --> A

  %% Direct client->server (no proxy) is also possible
  C -. gRPC tunnel .-> S

  classDef audit fill:#e8f5e9,stroke:#2e7d32,color:#1b5e20
  class S,A audit
```

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

Remote mode over gRPC
-
In remote mode, acp-gate instances communicate over a minimal gRPC tunnel that transports raw ACP JSON-RPC bytes. This lets you run the editor on one machine and the agent on another. You can also insert optional proxy hops if needed.
Modes:

1) Server with local agent and auditing (end server)
```
acp-gate -server 0 -config <cfg> -agent-name <name>
# or without config:
acp-gate -server 0 -agent-cmd <cmd> [-agent-arg ...]
```
Notes:
- Binds to an ephemeral port when -server 0 is used and logs the actual address.
- For each incoming connection, the server spawns the configured agent and audits traffic to the SQLite DB.

2) Client (no auditing)
```
acp-gate -connect <host:port>
```
Bridges your editor stdio to the remote server tunnel. No local agent is launched; no auditing is performed on the client.

3) Pure proxy server (no local agent, no auditing)
```
acp-gate -server 0 -connect <upstream_host:port>
```
Accepts client connections and relays them to another acp-gate server. Useful for inserting one or more hops, crossing network boundaries, or adding shims. Auditing is not performed on proxy hops; only the final server that launches the agent audits.

Chaining
-
You can insert one or more proxy hops between client and server as needed (no auditing on hops). See the Architecture diagram above.


Auditing behavior in remote mode:
- Only the end server that actually launches the downstream agent performs auditing.
- Client and pure-proxy hops do not audit.

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
- -server int
  Run in gRPC server mode on given port (0 for auto-bind; logs actual address)
- -connect string
  Run in gRPC client mode and connect to server at host:port

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

Security
-
The gRPC tunnel currently uses insecure transport for simplicity. If you need encryption and authentication, add TLS/mTLS and auth at deployment time. The protocol is stable and can be wrapped in standard gRPC security options.

Proto schema and Buf
-
- A minimal proto schema is provided at proto/acpgate/v1/tunnel.proto.
- Buf configuration (buf.yaml, buf.gen.yaml) is included for future typed codegen pipelines. The runtime uses a small manual codec/descriptor to keep the binary self-contained.

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