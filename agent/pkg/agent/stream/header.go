package stream

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"reflect"
	"unsafe"
)

const (
	versionSize  = 8
	hostSize     = 255
	portSize     = 5
	checksumSize = 4 // crc32 checksum

	headerSize = versionSize + 1 + hostSize + 1 + portSize + checksumSize
)

var version = [versionSize]byte{'0', '.', '1', '.', '0', 0, 0, 0}

var ErrInvalidHeader = errors.New("invalid header")

type StreamRequestHeader struct {
	Version [versionSize]byte

	HostLength byte
	Host       [hostSize]byte

	PortLength byte
	Port       [portSize]byte

	Checksum [checksumSize]byte
}

func init() {
	if headerSize != reflect.TypeFor[StreamRequestHeader]().Size() {
		panic("headerSize does not match the size of StreamRequestHeader")
	}
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
	header.HostLength = byte(len(host))
	copy(header.Host[:], host)
	header.PortLength = byte(len(port))
	copy(header.Port[:], port)
	header.updateChecksum()
	return header, nil
}

func ToHeader(buf [headerSize]byte) *StreamRequestHeader {
	return (*StreamRequestHeader)(unsafe.Pointer(&buf[0]))
}

func (h *StreamRequestHeader) GetHostPort() (string, string) {
	return string(h.Host[:h.HostLength]), string(h.Port[:h.PortLength])
}

func (h *StreamRequestHeader) Validate() bool {
	if h.Version != version {
		return false
	}
	if h.HostLength > hostSize {
		return false
	}
	if h.PortLength > portSize {
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
	return (*[headerSize - checksumSize]byte)(unsafe.Pointer(h))[:]
}

func (h *StreamRequestHeader) Bytes() []byte {
	return (*[headerSize]byte)(unsafe.Pointer(h))[:]
}
