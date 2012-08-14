package thrift

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	DefaultMaxFrameSize = 1024 * 1024
)

type ErrFrameTooBig struct {
	Size    int
	MaxSize int
}

func (e *ErrFrameTooBig) Error() string {
	return fmt.Sprintf("thrift: frame size while reading over allowed size (%d > %d)", e.Size, e.MaxSize)
}

type Flusher interface {
	Flush() error
}

type FramedReadWriteCloser struct {
	wrapped      io.ReadWriteCloser
	maxFrameSize int
	rbuf         *bytes.Buffer
	wbuf         *bytes.Buffer
}

func NewFramedReadWriteCloser(wrapped io.ReadWriteCloser, maxFrameSize int) *FramedReadWriteCloser {
	if maxFrameSize == 0 {
		maxFrameSize = DefaultMaxFrameSize
	}
	return &FramedReadWriteCloser{
		wrapped:      wrapped,
		maxFrameSize: maxFrameSize,
		rbuf:         &bytes.Buffer{},
		wbuf:         &bytes.Buffer{},
	}
}

func (f *FramedReadWriteCloser) Read(p []byte) (int, error) {
	if f.rbuf.Len() == 0 {
		f.rbuf.Reset()
		frameSize := uint32(0)
		if err := binary.Read(f.wrapped, binary.BigEndian, &frameSize); err != nil {
			return 0, err
		}
		if int(frameSize) > f.maxFrameSize {
			return 0, &ErrFrameTooBig{int(frameSize), f.maxFrameSize}
		}
		// TODO: Copy may return the full frame and still return an error. In that
		//       case we could return the asked for bytes to the caller (and the error).
		if _, err := io.CopyN(f.rbuf, f.wrapped, int64(frameSize)); err != nil {
			return 0, err
		}
	}
	n, err := f.rbuf.Read(p)
	return n, err
}

func (f *FramedReadWriteCloser) Write(p []byte) (int, error) {
	n, err := f.wbuf.Write(p)
	if err != nil {
		return n, err
	}
	if f.wbuf.Len() > f.maxFrameSize {
		return n, &ErrFrameTooBig{f.wbuf.Len(), f.maxFrameSize}
	}
	return n, nil
}

func (f *FramedReadWriteCloser) Close() error {
	return f.wrapped.Close()
}

func (f *FramedReadWriteCloser) Flush() error {
	frameSize := uint32(f.wbuf.Len())
	if frameSize > 0 {
		if err := binary.Write(f.wrapped, binary.BigEndian, frameSize); err != nil {
			return err
		}
		_, err := io.Copy(f.wrapped, f.wbuf)
		f.wbuf.Reset()
		return err
	}
	return nil
}
