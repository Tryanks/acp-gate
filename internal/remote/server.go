package remote

import (
    "fmt"
    "io"
    "os"
    "os/exec"

    acp "github.com/coder/acp-go-sdk"
    "acp-gate/internal/audit"
    "acp-gate/internal/proxy"
)

// ServerConfig provides downstream agent launch parameters and audit store.
type ServerConfig struct {
    Cmd  string
    Args []string
    Env  []string

    Store *audit.Store
}

// GateService implements GateServer.
type GateService struct {
    Cfg ServerConfig
}

func (s *GateService) Tunnel(stream Gate_TunnelServer) error {
    ctx := stream.Context()

    // Start downstream agent process per-connection.
    if s.Cfg.Cmd == "" {
        return fmt.Errorf("server misconfigured: empty agent command")
    }
    cmd := exec.CommandContext(ctx, s.Cfg.Cmd, s.Cfg.Args...)
    cmd.Env = append([]string{}, s.Cfg.Env...)
    cmd.Stderr = os.Stderr

    dsIn, err := cmd.StdinPipe()
    if err != nil { return err }
    dsOut, err := cmd.StdoutPipe()
    if err != nil { return err }
    if err := cmd.Start(); err != nil { return err }

    // Proxies with server-side auditing.
    proxyAgent := &proxy.ProxyAgent{}
    proxyAgent.SetStore(s.Cfg.Store)
    proxyClient := &proxy.ProxyClient{}
    proxyClient.SetStore(s.Cfg.Store)

    // Upstream is the remote client via gRPC stream.
    upWriter := NewStreamWriter(stream.Send)
    upReader := NewStreamReader(stream.Recv)
    upstreamConn := acp.NewAgentSideConnection(proxyAgent, upWriter, upReader)
    proxyClient.SetUpstream(upstreamConn)

    // Downstream is the real agent process.
    downstreamConn := acp.NewClientSideConnection(proxyClient, dsIn, dsOut)
    proxyAgent.SetDownstream(downstreamConn)

    // Lifecycle: wait for either side to close or process exit.
    waitCh := make(chan error, 1)
    go func() { waitCh <- cmd.Wait() }()

    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-upstreamConn.Done():
        return nil
    case <-downstreamConn.Done():
        return nil
    case err := <-waitCh:
        if err == io.EOF {
            return nil
        }
        return err
    }
}
