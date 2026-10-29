package gateway

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"lam.local/lam2/internal/fault"
	"strings"
)

type Frame struct {
	Event string
	Data  []byte
}

var errTerminal = errors.New("terminal event sent")

// ReadSSE operates on event frames, never on HTTP chunk boundaries.
func ReadSSE(r io.Reader, maxBytes int, emit func(Frame) error) error {
	br := bufio.NewReaderSize(r, 4096)
	var event string
	var data []byte
	size := 0
	dispatch := func() error {
		if len(data) == 0 {
			event = ""
			size = 0
			return nil
		}
		f := Frame{event, bytes.TrimSuffix(data, []byte("\n"))}
		event = ""
		data = nil
		size = 0
		return emit(f)
	}
	for {
		var line []byte
		for {
			part, e := br.ReadSlice('\n')
			if len(line)+len(part)+size > maxBytes {
				return fault.New("SSE_FRAME_TOO_LARGE", "SSE event exceeds frame limit")
			}
			line = append(line, part...)
			if e == bufio.ErrBufferFull {
				continue
			}
			if e != nil && e != io.EOF {
				return e
			}
			if e == io.EOF && len(line) == 0 {
				return dispatch()
			}
			line = bytes.TrimRight(line, "\r\n")
			size += len(line) + 1
			if len(line) == 0 {
				if err := dispatch(); err != nil {
					return err
				}
			} else if line[0] != ':' {
				name, value, found := strings.Cut(string(line), ":")
				if found {
					value = strings.TrimPrefix(value, " ")
				}
				switch name {
				case "event":
					event = value
				case "data":
					data = append(data, []byte(value)...)
					data = append(data, '\n')
				}
			}
			if e == io.EOF {
				return dispatch()
			}
			break
		}
	}
}
