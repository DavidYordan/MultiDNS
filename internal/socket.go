package internal

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func createTransparentUDPSocket(addr *net.UDPAddr) (*net.UDPConn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		fmt.Printf("Failed to create socket: %v\n", err)
		return nil, err
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		fmt.Printf("Failed to set SO_REUSEADDR: %v\n", err)
		syscall.Close(fd)
		return nil, err
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil {
		fmt.Printf("Failed to set IP_TRANSPARENT: %v\n", err)
		syscall.Close(fd)
		return nil, err
	}

	sa := &syscall.SockaddrInet4{Port: addr.Port}
	copy(sa.Addr[:], addr.IP.To4())

	if err := syscall.Bind(fd, sa); err != nil {
		fmt.Printf("Failed to bind socket: %v\n", err)
		syscall.Close(fd)
		return nil, err
	}

	file := os.NewFile(uintptr(fd), "")
	defer file.Close()

	conn, err := net.FilePacketConn(file)
	if err != nil {
		fmt.Printf("Failed to create FilePacketConn: %v\n", err)
		return nil, err
	}

	return conn.(*net.UDPConn), nil
}
