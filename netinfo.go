package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"sort"
	"time"
)

// detectLocalNets returns the IPv4 networks of the NAS's own interfaces.
func detectLocalNets() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones < 8 {
				continue
			}
			n := (&net.IPNet{IP: ipn.IP.Mask(ipn.Mask), Mask: ipn.Mask}).String()
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// lanAddresses lists the NAS's own private IPv4 addresses (for phone setup).
func lanAddresses() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() || ipn.IP.IsLinkLocalUnicast() {
			continue
		}
		if ipn.IP.IsPrivate() {
			out = append(out, ipn.IP.String())
		}
	}
	sort.Strings(out)
	return out
}

var stunServers = []string{"stun.l.google.com:19302", "stun.cloudflare.com:3478", "stun1.l.google.com:19302"}

// stunPublicIP asks public STUN servers for the NAS's public IPv4 address.
func stunPublicIP(ctx context.Context) (string, error) {
	var lastErr error
	for _, srv := range stunServers {
		ip, err := stunQuery(ctx, srv)
		if err == nil {
			return ip, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("STUN yanıtı yok")
	}
	return "", lastErr
}

func stunQuery(ctx context.Context, server string) (string, error) {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "udp4", server)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:2], 0x0001) // Binding Request
	binary.BigEndian.PutUint32(req[4:8], 0x2112A442)
	_, _ = rand.Read(req[8:20])
	resp := make([]byte, 1024)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := conn.Write(req); err != nil {
			return "", err
		}
		_ = conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		n, err := conn.Read(resp)
		if err != nil {
			continue
		}
		if ip := parseSTUN(resp[:n], req[8:20]); ip != "" {
			return ip, nil
		}
	}
	return "", errors.New("STUN yanıtı yok: " + server)
}

func parseSTUN(b, txid []byte) string {
	if len(b) < 20 || binary.BigEndian.Uint16(b[0:2]) != 0x0101 || string(b[8:20]) != string(txid) {
		return ""
	}
	length := int(binary.BigEndian.Uint16(b[2:4]))
	attrs := b[20:]
	if length < len(attrs) {
		attrs = attrs[:length]
	}
	var mapped string
	for len(attrs) >= 4 {
		t := binary.BigEndian.Uint16(attrs[0:2])
		l := int(binary.BigEndian.Uint16(attrs[2:4]))
		if 4+l > len(attrs) {
			break
		}
		v := attrs[4 : 4+l]
		if len(v) >= 8 && v[1] == 0x01 { // IPv4
			ip := net.IPv4(v[4], v[5], v[6], v[7])
			switch t {
			case 0x0020: // XOR-MAPPED-ADDRESS
				return net.IPv4(v[4]^0x21, v[5]^0x12, v[6]^0xA4, v[7]^0x42).String()
			case 0x0001: // MAPPED-ADDRESS
				mapped = ip.String()
			}
		}
		pad := (4 - l%4) % 4
		attrs = attrs[4+l+pad:]
		if len(attrs) < 0 {
			break
		}
	}
	return mapped
}
