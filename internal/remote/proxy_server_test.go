package remote

import (
    "context"
    "net"
    "testing"
    "time"

    "google.golang.org/grpc"
)

// echoGate is a simple GateServer that echoes any received bytes back to the client.
type echoGate struct{}

func (echoGate) Tunnel(stream Gate_TunnelServer) error {
    for {
        b, err := stream.Recv()
        if err != nil {
            return err
        }
        if err := stream.Send(b); err != nil {
            return err
        }
    }
}

func startGRPCServer(t *testing.T, srv GateServer) (addr string, stop func()) {
    t.Helper()
    lis, err := net.Listen("tcp", ":0")
    if err != nil {
        t.Fatalf("listen: %v", err)
    }
    grpcServer := grpc.NewServer(grpc.ForceServerCodec(RawCodec))
    RegisterGateServer(grpcServer, srv)
    go grpcServer.Serve(lis)
    return lis.Addr().String(), func() { grpcServer.GracefulStop(); _ = lis.Close() }
}

func TestPureProxyServerRelays(t *testing.T) {
    t.Parallel()
    // Start upstream echo server
    upstreamAddr, upstreamStop := startGRPCServer(t, echoGate{})
    defer upstreamStop()

    // Start proxy server that connects to upstream
    proxySrv := &GateService{Cfg: ServerConfig{ConnectAddr: upstreamAddr}}
    proxyAddr, proxyStop := startGRPCServer(t, proxySrv)
    defer proxyStop()

    // Connect a client to proxy server
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    conn, err := grpc.DialContext(ctx, proxyAddr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(grpc.ForceCodec(RawCodec)))
    if err != nil {
        t.Fatalf("dial proxy: %v", err)
    }
    defer conn.Close()

    cli := NewGateClient(conn)
    stream, err := cli.Tunnel(ctx)
    if err != nil {
        t.Fatalf("open tunnel via proxy: %v", err)
    }
    defer stream.CloseSend()

    // Send payloads and expect same back
    payloads := [][]byte{[]byte("one"), []byte("two"), []byte("three")}
    for _, p := range payloads {
        if err := stream.Send(p); err != nil {
            t.Fatalf("send: %v", err)
        }
        got, err := stream.Recv()
        if err != nil {
            t.Fatalf("recv: %v", err)
        }
        if string(got) != string(p) {
            t.Fatalf("mismatch: got %q want %q", string(got), string(p))
        }
    }
}
