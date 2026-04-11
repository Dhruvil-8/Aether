package node

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

var (
	ErrTorProxyRejected    = errors.New("tor socks proxy rejected connection")
	ErrTorProxyUnreachable = errors.New("tor socks proxy unreachable")
)

type TorTransport struct {
	SOCKSAddress string
	Timeout      time.Duration
}

func NewTorTransport(addr string) *TorTransport {
	return &TorTransport{SOCKSAddress: addr, Timeout: 5 * time.Second}
}

func NewTorTransportWithTimeout(addr string, timeout time.Duration) *TorTransport {
	transport := NewTorTransport(addr)
	if timeout > 0 {
		transport.Timeout = timeout
	}
	return transport
}

func (t *TorTransport) Dial(network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := net.DialTimeout("tcp", t.SOCKSAddress, timeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTorProxyUnreachable, err)
	}

	if err := negotiateSOCKS5(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := sendConnectRequest(conn, address); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func CheckTorSOCKS(address string, timeout time.Duration) error {
	conn, err := NewTorTransportWithTimeout(address, timeout).Dial("tcp", "example.com:80")
	if err != nil {
		return err
	}
	return conn.Close()
}

func negotiateSOCKS5(conn net.Conn) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		return ErrTorProxyRejected
	}
	return nil
}

func sendConnectRequest(conn net.Conn, address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	portNum, err := strconv.Atoi(portText)
	if err != nil {
		return err
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid port: %d", portNum)
	}

	hostBytes := []byte(host)
	req := make([]byte, 0, 6+len(hostBytes))
	req = append(req, 0x05, 0x01, 0x00, 0x03, byte(len(hostBytes)))
	req = append(req, hostBytes...)
	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, uint16(portNum))
	req = append(req, portBuf...)
	if _, err := conn.Write(req); err != nil {
		return err
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	if head[0] != 0x05 || head[1] != 0x00 {
		return ErrTorProxyRejected
	}

	addrLen := 0
	switch head[3] {
	case 0x01:
		addrLen = 4
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return err
		}
		addrLen = int(lenBuf[0])
	case 0x04:
		addrLen = 16
	default:
		return ErrTorProxyRejected
	}
	discard := make([]byte, addrLen+2)
	_, err = io.ReadFull(conn, discard)
	return err
}
