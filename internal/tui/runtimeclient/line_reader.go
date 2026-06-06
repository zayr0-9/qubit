package runtimeclient

import (
	"bytes"
	"fmt"
	"io"
)

const maxRuntimeLineBytes = 64 * 1024 * 1024

type runtimeLineReader struct {
	r   io.Reader
	buf []byte
}

func newRuntimeLineReader(r io.Reader) *runtimeLineReader {
	return &runtimeLineReader{r: r, buf: make([]byte, 0, 64*1024)}
}

func (r *runtimeLineReader) readLine() ([]byte, error) {
	for {
		if idx := bytes.IndexByte(r.buf, '\n'); idx >= 0 {
			line := append([]byte(nil), r.buf[:idx]...)
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			r.buf = r.buf[idx+1:]
			return line, nil
		}
		if len(r.buf) > maxRuntimeLineBytes {
			return nil, fmt.Errorf("runtime event line exceeded %d bytes", maxRuntimeLineBytes)
		}
		tmp := make([]byte, 64*1024)
		n, err := r.r.Read(tmp)
		if n > 0 {
			r.buf = append(r.buf, tmp[:n]...)
			continue
		}
		if err != nil {
			if err == io.EOF && len(r.buf) > 0 {
				line := append([]byte(nil), r.buf...)
				if len(line) > 0 && line[len(line)-1] == '\r' {
					line = line[:len(line)-1]
				}
				r.buf = nil
				return line, nil
			}
			return nil, err
		}
	}
}

func previewRuntimeLine(line []byte) string {
	const limit = 4096
	if len(line) <= limit {
		return string(line)
	}
	return fmt.Sprintf("%s… [runtime event line truncated in log: %d bytes]", string(line[:limit]), len(line))
}
