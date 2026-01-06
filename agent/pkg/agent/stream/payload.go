package stream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"unsafe"
)

const (
	versionSize  = 8
	hostSize     = 255
	portSize     = 5
	checksumSize = 4 // crc32 checksum

	headerSize = versionSize + hostSize + portSize + checksumSize
)

var version = [versionSize]byte{'0', '.', '1', '.', '0', 0, 0, 0}

var ErrInvalidHeader = errors.New("invalid header")

type StreamRequestHeader struct {
	Version  [versionSize]byte
	Host     [hostSize]byte
	Port     [portSize]byte
	Checksum [checksumSize]byte
}

type StreamRequestPayload struct {
	StreamRequestHeader
	Data []byte
}

func NewStreamRequestHeader(host, port string) (*StreamRequestHeader, error) {
	if len(host) > hostSize {
		return nil, fmt.Errorf("host is too long: max %d characters, got %d", hostSize, len(host))
	}
	if len(port) > portSize {
		return nil, fmt.Errorf("port is too long: max %d characters, got %d", portSize, len(port))
	}
	header := &StreamRequestHeader{}
	copy(header.Version[:], version[:])
	copy(header.Host[:], host)
	copy(header.Port[:], port)
	header.updateChecksum()
	return header, nil
}

func ToHeader(buf [headerSize]byte) *StreamRequestHeader {
	return (*StreamRequestHeader)(unsafe.Pointer(&buf[0]))
}

// WriteTo implements the io.WriterTo interface.
func (p *StreamRequestPayload) WriteTo(w io.Writer) (n int64, err error) {
	n1, err := w.Write(p.StreamRequestHeader.Bytes())
	if err != nil {
		return
	}
	if len(p.Data) == 0 {
		return int64(n1), nil
	}

	n2, err := w.Write(p.Data)
	if err != nil {
		return
	}
	return int64(n1) + int64(n2), nil
}

func (h *StreamRequestHeader) GetHostPort() (string, string) {
	hostEnd := bytes.IndexByte(h.Host[:], 0)
	portEnd := bytes.IndexByte(h.Port[:], 0)
	if hostEnd == -1 {
		hostEnd = hostSize
	}
	if portEnd == -1 {
		portEnd = portSize
	}
	return string(h.Host[:hostEnd]), string(h.Port[:portEnd])
}

func (h *StreamRequestHeader) Validate() bool {
	if h.Version != version {
		return false
	}
	return h.validateChecksum()
}

func (h *StreamRequestHeader) updateChecksum() {
	checksum := crc32.ChecksumIEEE(h.BytesWithoutChecksum())
	binary.BigEndian.PutUint32(h.Checksum[:], checksum)
}

func (h *StreamRequestHeader) validateChecksum() bool {
	checksum := crc32.ChecksumIEEE(h.BytesWithoutChecksum())
	return checksum == binary.BigEndian.Uint32(h.Checksum[:])
}

func (h *StreamRequestHeader) BytesWithoutChecksum() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(h)), headerSize-checksumSize)
}

func (h *StreamRequestHeader) Bytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(h)), headerSize)
}
