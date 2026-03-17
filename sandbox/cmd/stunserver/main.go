// Lightweight STUN server for sandbox testing.
// Responds to STUN Binding Requests with XOR-MAPPED-ADDRESS
// reflecting the client's observed address.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
)

const (
	stunBindingRequest  = 0x0001
	stunBindingResponse = 0x0101
	stunMagicCookie     = 0x2112A442
	stunHeaderSize      = 20
	stunAttrXorMapped   = 0x0020
)

func main() {
	addr := flag.String("addr", "0.0.0.0:3478", "listen address")
	flag.Parse()

	conn, err := net.ListenPacket("udp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	fmt.Printf("STUN server listening on %s\n", conn.LocalAddr())

	buf := make([]byte, 1500)
	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("read: %v", err)
			continue
		}

		if n < stunHeaderSize {
			continue
		}

		msgType := binary.BigEndian.Uint16(buf[0:2])
		if msgType != stunBindingRequest {
			continue
		}

		magic := binary.BigEndian.Uint32(buf[4:8])
		if magic != stunMagicCookie {
			continue
		}

		txID := make([]byte, 12)
		copy(txID, buf[8:20])

		udpAddr := remote.(*net.UDPAddr)
		resp := buildResponse(txID, udpAddr.IP, udpAddr.Port)

		if _, err := conn.WriteTo(resp, remote); err != nil {
			log.Printf("write: %v", err)
		}
	}
}

func buildResponse(txID []byte, ip net.IP, port int) []byte {
	resp := make([]byte, stunHeaderSize+12)

	// Header
	binary.BigEndian.PutUint16(resp[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(resp[2:4], 12) // attribute payload length
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
	copy(resp[8:20], txID)

	// XOR-MAPPED-ADDRESS attribute
	binary.BigEndian.PutUint16(resp[20:22], stunAttrXorMapped)
	binary.BigEndian.PutUint16(resp[22:24], 8) // value length
	resp[24] = 0    // reserved
	resp[25] = 0x01 // IPv4

	// XOR port with magic cookie high 16 bits
	xorPort := uint16(port) ^ uint16(stunMagicCookie>>16) //nolint:gosec // port from net.UDPAddr is always 0-65535
	binary.BigEndian.PutUint16(resp[26:28], xorPort)

	// XOR IP with magic cookie
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4(127, 0, 0, 1).To4()
	}
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		resp[28+i] = ip4[i] ^ magicBytes[i]
	}

	return resp
}
