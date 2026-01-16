package remote

import (
    "fmt"
    "io"
    "os"
    "os/exec"

    acp "github.com/coder/acp-go-sdk"
    "acp-gate/internal/audit"
    "acp-gate/internal/proxy"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

// ServerConfig provides downstream agent launch parameters and audit store.
type ServerConfig struct {
    Cmd  string
    Args []string
    Env  []string

    Store *audit.Store

    // ConnectAddr, if non-empty, enables pure-proxy mode: instead of launching
    // a local downstream agent process, the server will dial another acp-gate
    // instance at this address and relay bytes between the two tunnel streams.
    ConnectAddr string
}

// GateService implements GateServer.
type GateService struct {
    Cfg ServerConfig
}

func (s *GateService) Tunnel(stream Gate_TunnelServer) error {
    ctx := stream.Context()

    // Pure proxy mode: forward to another server instead of spawning a process.
    if s.Cfg.ConnectAddr != "" {
        // Dial upstream server and open a tunnel
        conn, err := grpc.DialContext(ctx, s.Cfg.ConnectAddr,
            grpc.WithTransportCredentials(insecure.NewCredentials()),
            grpc.WithDefaultCallOptions(grpc.ForceCodec(RawCodec)))
        if err != nil {
            return fmt.Errorf("dial upstream %q: %w", s.Cfg.ConnectAddr, err)
        }
        defer conn.Close()

        cli := NewGateClient(conn)
        upStream, err := cli.Tunnel(ctx)
        if err != nil {
            return fmt.Errorf("open upstream tunnel: %w", err)
        }

        // Bridge bytes between streams in both directions.
        // downstream (client->server) -> upstream (server->next)
        errCh := make(chan error, 2)

        go func() {
            // Copy from incoming stream to upstream tunnel
            _, err := io.Copy(NewStreamWriter(upStream.Send), NewStreamReader(stream.Recv))
            // Close send on upstream to signal EOF
            _ = upStream.CloseSend()
            errCh <- err
        }()

        go func() {
            // Copy from upstream tunnel back to the incoming stream
            _, err := io.Copy(NewStreamWriter(stream.Send), NewStreamReader(upStream.Recv))
            errCh <- err
        }()

        select {
        case <-ctx.Done():
            return ctx.Err()
        case err := <-errCh:
            if err == io.EOF || err == nil {
                return nil
            }
            return err
        case err := <-errCh:
            if err == io.EOF || err == nil {
                return nil
            }
            return err
        }
    }

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
