package tftpserver

import (
	"embed"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/pin/tftp/v3"
)

//go:embed ipxebin/*
var ipxeFS embed.FS

var files = map[string]string{
	"undionly.kpxe":  "ipxebin/undionly.kpxe",
	"ipxe.efi":      "ipxebin/ipxe.efi",
	"ipxe-arm64.efi": "ipxebin/ipxe-arm64.efi",
}

func readHandler(filename string, rf io.ReaderFrom) error {
	path, ok := files[filename]
	if !ok {
		log.Printf("tftp: file not found: %s", filename)
		return fmt.Errorf("file not found: %s", filename)
	}

	data, err := ipxeFS.ReadFile(path)
	if err != nil {
		log.Printf("tftp: error reading embedded file %s: %v", path, err)
		return fmt.Errorf("read embedded file: %w", err)
	}

	rf.(tftp.OutgoingTransfer).SetSize(int64(len(data)))

	n, err := rf.ReadFrom(newBytesReader(data))
	if err != nil {
		log.Printf("tftp: error sending %s: %v", filename, err)
		return err
	}
	log.Printf("tftp: sent %s (%d bytes)", filename, n)
	return nil
}

func GetIPXEBinary(name string) ([]byte, error) {
	path, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("unknown iPXE binary: %s", name)
	}
	return ipxeFS.ReadFile(path)
}

func NewServer(addr string) *tftp.Server {
	s := tftp.NewServer(readHandler, nil)
	s.SetTimeout(5 * time.Second)
	s.SetRetries(3)
	return s
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
