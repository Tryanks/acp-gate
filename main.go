package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	acp "github.com/coder/acp-go-sdk"
	"acp-gate/internal/audit"
	"acp-gate/internal/proxy"
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
		agentCmd    string
		agentArgs   multiFlag
	)

	flag.StringVar(&auditDBPath, "audit-db", "audit.sqlite", "path to SQLite audit DB")
	flag.StringVar(&agentCmd, "agent-cmd", "", "downstream real agent command")
	flag.Var(&agentArgs, "agent-arg", "argument for downstream agent (repeatable)")
	flag.Parse()

	if agentCmd == "" {
		fmt.Fprintln(os.Stderr, "missing required flag: -agent-cmd")
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := audit.Open(ctx, auditDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open audit db: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	downstream := exec.CommandContext(ctx, agentCmd, agentArgs...)
	downstream.Stderr = os.Stderr

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
