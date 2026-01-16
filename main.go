package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "net"
    "os"
    "os/exec"
    "os/signal"
    "syscall"

    acp "github.com/coder/acp-go-sdk"
    "acp-gate/internal/audit"
    "acp-gate/internal/config"
    "acp-gate/internal/proxy"
    "acp-gate/internal/remote"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/encoding"
)

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
    var (
        auditDBPath string
        cfgPath     string
        agentName   string
        agentCmd    string
        agentArgs   multiFlag
        servePort   int
        connectAddr string
    )

    flag.StringVar(&auditDBPath, "audit-db", "audit.sqlite", "path to SQLite audit DB")
    flag.StringVar(&cfgPath, "config", "", "path to JSON config file with agent_servers (default: ~/.config/.acp-gate/config.json if present)")
    flag.StringVar(&agentName, "agent-name", "", "agent server name from config to use")
    flag.StringVar(&agentCmd, "agent-cmd", "", "downstream real agent command")
    flag.Var(&agentArgs, "agent-arg", "argument for downstream agent (repeatable)")
    flag.IntVar(&servePort, "server", -1, "run in server mode on given port (0 for auto)")
    flag.StringVar(&connectAddr, "connect", "", "run in client mode, connect to server at host:port")
    flag.Parse()

    var cfg config.Config
    var err error
    if cfgPath == "" {
        if p, ok := config.FindExistingDefaultConfig(); ok {
            cfgPath = p
        }
    }
    if cfgPath != "" {
        cfg, err = config.Load(cfgPath)
        if err != nil {
            fmt.Fprintf(os.Stderr, "load config: %v\n", err)
            os.Exit(2)
        }
    }

    logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
    slog.SetDefault(logger)

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Register raw codec for gRPC tunnel.
    encoding.RegisterCodec(remote.RawCodec)

    // Resolve downstream command/args/env
    resolvedCmd := agentCmd
    resolvedArgs := []string(agentArgs)
    var resolvedEnv []string
    if cfgPath != "" {
        rc, ra, renv, rerr := config.Resolve(cfg, agentName, agentCmd, resolvedArgs, os.Environ())
        if rerr != nil {
            fmt.Fprintf(os.Stderr, "%v\n", rerr)
            os.Exit(2)
        }
        resolvedCmd, resolvedArgs, resolvedEnv = rc, ra, renv
    } else {
        if resolvedCmd == "" {
            fmt.Fprintln(os.Stderr, "missing required flag: -agent-cmd (or provide -config or place it at ~/.config/.acp-gate/config.json)")
            os.Exit(2)
        }
        resolvedEnv = os.Environ()
    }

    // MODE SELECTION
    if servePort >= 0 {
        // SERVER MODE
        // Open audit store (server-side only)
        store, err := audit.Open(ctx, auditDBPath)
        if err != nil {
            fmt.Fprintf(os.Stderr, "open audit db: %v\n", err)
            os.Exit(1)
        }
        defer store.Close()

        lis, err := net.Listen("tcp", fmt.Sprintf(":%d", servePort))
        if err != nil {
            fmt.Fprintf(os.Stderr, "listen: %v\n", err)
            os.Exit(1)
        }
        addr := lis.Addr().String()
        slog.Info("acp-gate server listening", "addr", addr)

        grpcServer := grpc.NewServer(grpc.ForceServerCodec(remote.RawCodec))
        remote.RegisterGateServer(grpcServer, &remote.GateService{Cfg: remote.ServerConfig{
            Cmd:   resolvedCmd,
            Args:  resolvedArgs,
            Env:   resolvedEnv,
            Store: store,
        }})

        go func() {
            if err := grpcServer.Serve(lis); err != nil {
                slog.Error("grpc serve error", "err", err)
                stop()
            }
        }()

        <-ctx.Done()
        grpcServer.GracefulStop()
        return
    }

    if connectAddr != "" {
        // CLIENT MODE (no local auditing)
        conn, err := grpc.DialContext(ctx, connectAddr,
            grpc.WithTransportCredentials(insecure.NewCredentials()),
            grpc.WithDefaultCallOptions(grpc.ForceCodec(remote.RawCodec)))
        if err != nil {
            fmt.Fprintf(os.Stderr, "dial server: %v\n", err)
            os.Exit(1)
        }
        defer conn.Close()

        cli := remote.NewGateClient(conn)
        stream, err := cli.Tunnel(ctx)
        if err != nil {
            fmt.Fprintf(os.Stderr, "open tunnel: %v\n", err)
            os.Exit(1)
        }

        // Prepare proxy without audit store.
        proxyAgent := &proxy.ProxyAgent{}
        proxyClient := &proxy.ProxyClient{}

        // Upstream: Editor via stdio
        upstreamConn := acp.NewAgentSideConnection(proxyAgent, os.Stdout, os.Stdin)
        proxyClient.SetUpstream(upstreamConn)

        // Downstream: remote gRPC tunnel
        downWriter := remote.NewStreamWriter(stream.Send)
        downReader := remote.NewStreamReader(stream.Recv)
        downstreamConn := acp.NewClientSideConnection(proxyClient, downWriter, downReader)
        proxyAgent.SetDownstream(downstreamConn)

        select {
        case <-upstreamConn.Done():
        case <-downstreamConn.Done():
        case <-ctx.Done():
        }
        _ = stream.CloseSend()
        return
    }

    // LOCAL MODE (legacy): direct process spawn with local auditing

    store, err := audit.Open(ctx, auditDBPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "open audit db: %v\n", err)
        os.Exit(1)
    }
    defer store.Close()

    downstream := exec.CommandContext(ctx, resolvedCmd, resolvedArgs...)
    downstream.Stderr = os.Stderr
    downstream.Env = resolvedEnv

    dsIn, err := downstream.StdinPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "downstream stdin: %v\n", err)
        os.Exit(1)
	}
	dsOut, err := downstream.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "downstream stdout: %v\n", err)
		os.Exit(1)
	}

	if err := downstream.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start downstream agent: %v\n", err)
		os.Exit(1)
	}

	// 2. Prepare proxy and connections.
	proxyAgent := &proxy.ProxyAgent{}
	proxyAgent.SetStore(store)

	proxyClient := &proxy.ProxyClient{}
	proxyClient.SetStore(store)

	// Connect to Editor (Upstream)
	// Editor writes to our Stdin, reads from our Stdout.
	// From our perspective: peerInput is os.Stdout, peerOutput is os.Stdin.
	upstreamConn := acp.NewAgentSideConnection(proxyAgent, os.Stdout, os.Stdin)
	proxyClient.SetUpstream(upstreamConn)

	// Connect to Real Agent (Downstream)
	// Agent reads from dsIn, writes to dsOut.
	// From our perspective: peerInput is dsIn, peerOutput is dsOut.
	downstreamConn := acp.NewClientSideConnection(proxyClient, dsIn, dsOut)
	proxyAgent.SetDownstream(downstreamConn)

	// 3. Lifecycle management.
	waitCh := make(chan error, 1)

	go func() {
		waitCh <- downstream.Wait()
	}()

	select {
	case <-upstreamConn.Done():
		// Editor closed connection
	case <-downstreamConn.Done():
		// Agent closed connection
	case err := <-waitCh:
		if err != nil {
			fmt.Fprintf(os.Stderr, "downstream agent exited with error: %v\n", err)
		}
	case <-ctx.Done():
		// Signal received
	}
}
