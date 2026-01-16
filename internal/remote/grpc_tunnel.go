package remote

import (
    "context"
    "errors"
    "io"
    "sync"

    "google.golang.org/grpc"
)

// rawCodec is a trivial gRPC codec that passes []byte through unchanged.
// This lets us implement a generic byte-stream tunnel over gRPC without
// requiring generated protobuf code.
type rawCodec struct{}

func (rawCodec) Name() string { return "raw" }
func (rawCodec) Marshal(v interface{}) ([]byte, error) {
    b, ok := v.([]byte)
    if !ok {
        return nil, errors.New("raw codec expects []byte")
    }
    return b, nil
}
func (rawCodec) Unmarshal(data []byte, v interface{}) error {
    p, ok := v.(*[]byte)
    if !ok {
        return errors.New("raw codec expects *[]byte")
    }
    *p = append((*p)[:0], data...)
    return nil
}

var RawCodec = rawCodec{}

const (
    GateServiceName = "acpgate.v1.Gate"
    GateTunnelMethod = "/acpgate.v1.Gate/Tunnel"
)

// Gate_TunnelServer is the server-side stream for Tunnel.
type Gate_TunnelServer interface {
    Send(b []byte) error
    Recv() ([]byte, error)
    grpc.ServerStream
}

// GateServer defines our service entry points.
type GateServer interface {
    Tunnel(Gate_TunnelServer) error
}

// RegisterGateServer registers the Gate service on the gRPC server.
func RegisterGateServer(s *grpc.Server, srv GateServer) {
    s.RegisterService(&grpc.ServiceDesc{
        ServiceName: GateServiceName,
        HandlerType: (*GateServer)(nil),
        Streams: []grpc.StreamDesc{
            {
                StreamName:    "Tunnel",
                Handler:       _Gate_Tunnel_Handler,
                ServerStreams: true,
                ClientStreams: true,
            },
        },
        Methods: []grpc.MethodDesc{},
    }, srv)
}

// server stream adapter
func _Gate_Tunnel_Handler(srv interface{}, stream grpc.ServerStream) error {
    s := &gateTunnelServer{ServerStream: stream}
    return srv.(GateServer).Tunnel(s)
}

type gateTunnelServer struct{ grpc.ServerStream }

func (s *gateTunnelServer) Send(b []byte) error { return s.ServerStream.SendMsg(b) }
func (s *gateTunnelServer) Recv() ([]byte, error) {
    var b []byte
    if err := s.ServerStream.RecvMsg(&b); err != nil {
        return nil, err
    }
    return b, nil
}

// Client API
type GateClient interface {
    Tunnel(ctx context.Context, opts ...grpc.CallOption) (Gate_TunnelClient, error)
}

type gateClient struct{ cc *grpc.ClientConn }

func NewGateClient(cc *grpc.ClientConn) GateClient { return &gateClient{cc: cc} }

type Gate_TunnelClient interface {
    Send(b []byte) error
    Recv() ([]byte, error)
    CloseSend() error
    grpc.ClientStream
}

func (c *gateClient) Tunnel(ctx context.Context, opts ...grpc.CallOption) (Gate_TunnelClient, error) {
    // Ensure raw codec is used by default
    opts = append([]grpc.CallOption{grpc.ForceCodec(RawCodec)}, opts...)
    desc := &grpc.StreamDesc{StreamName: "Tunnel", ServerStreams: true, ClientStreams: true}
    cs, err := c.cc.NewStream(ctx, desc, GateTunnelMethod, opts...)
    if err != nil {
        return nil, err
    }
    return &gateTunnelClient{ClientStream: cs}, nil
}

type gateTunnelClient struct{ grpc.ClientStream }

func (c *gateTunnelClient) Send(b []byte) error { return c.ClientStream.SendMsg(b) }
func (c *gateTunnelClient) Recv() ([]byte, error) {
    var b []byte
    if err := c.ClientStream.RecvMsg(&b); err != nil {
        return nil, err
    }
    return b, nil
}
func (c *gateTunnelClient) CloseSend() error { return c.ClientStream.CloseSend() }

// streamReader adapts a Gate stream Recv() into an io.Reader with internal buffering.
type streamReader struct {
    recv func() ([]byte, error)
    buf  []byte
    mu   sync.Mutex
}

func NewStreamReader(recv func() ([]byte, error)) io.Reader { return &streamReader{recv: recv} }

func (r *streamReader) Read(p []byte) (int, error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    for len(r.buf) == 0 {
        b, err := r.recv()
        if err != nil {
            if err == io.EOF {
                return 0, io.EOF
            }
            return 0, err
        }
        r.buf = append(r.buf, b...)
    }
    n := copy(p, r.buf)
    r.buf = r.buf[n:]
    return n, nil
}

// streamWriter adapts a Gate stream Send() into an io.Writer.
type streamWriter struct{ send func([]byte) error }

func NewStreamWriter(send func([]byte) error) io.Writer { return &streamWriter{send: send} }

func (w *streamWriter) Write(p []byte) (int, error) {
    if err := w.send(append([]byte(nil), p...)); err != nil {
        return 0, err
    }
    return len(p), nil
}
