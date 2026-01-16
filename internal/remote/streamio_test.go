package remote

import (
    "bytes"
    "io"
    "testing"
)

func TestStreamWriterAndReader(t *testing.T) {
    // Channel-backed fake stream
    ch := make(chan []byte, 4)

    w := NewStreamWriter(func(b []byte) error { ch <- b; return nil })
    r := NewStreamReader(func() ([]byte, error) {
        b, ok := <-ch
        if !ok {
            return nil, io.EOF
        }
        return b, nil
    })

    payloads := [][]byte{
        []byte("hello "),
        []byte("world"),
        []byte("!"),
    }
    total := []byte("hello world!")

    // Write payloads
    for _, p := range payloads {
        if n, err := w.Write(p); err != nil || n != len(p) {
            t.Fatalf("write err=%v n=%d", err, n)
        }
    }

    // Read back in smaller chunks to exercise buffering
    buf := make([]byte, 0, len(total))
    tmp := make([]byte, 3)
    for len(buf) < len(total) {
        n, err := r.Read(tmp)
        if err != nil && err != io.EOF {
            t.Fatalf("read err: %v", err)
        }
        buf = append(buf, tmp[:n]...)
        if err == io.EOF {
            break
        }
        if len(buf) >= len(total) {
            break
        }
    }

    if !bytes.Equal(buf, total) {
        t.Fatalf("roundtrip mismatch: got %q want %q", string(buf), string(total))
    }
}
