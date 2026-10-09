package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	geckoLinkGatewayMAC = "30:1F:9A:D1:45:49"
	gldpPort            = 9999
	geckoLinkRESTPort   = 8889
	gatewayRefresh      = 30 * time.Second
)

type gldpResponse struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
	Type     string `json:"type"`
	Gateway  struct {
		MAC string `json:"mac"`
	} `json:"gateway"`
}

type gatewayResolver struct {
	mu         sync.Mutex
	baseURL    string
	resolvedAt time.Time
	discover   func(context.Context) (string, error)
}

func newGatewayResolver() *gatewayResolver {
	return &gatewayResolver{discover: func(ctx context.Context) (string, error) {
		return discoverGeckoLinkGateway(ctx, geckoLinkGatewayMAC, gldpPort, geckoLinkRESTPort)
	}}
}

func staticGatewayResolver(baseURL string) *gatewayResolver {
	return &gatewayResolver{baseURL: strings.TrimRight(baseURL, "/"), resolvedAt: time.Now()}
}

func (r *gatewayResolver) URL(ctx context.Context, force bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !force && r.baseURL != "" && time.Since(r.resolvedAt) < gatewayRefresh {
		return r.baseURL, nil
	}
	if r.discover == nil {
		if r.baseURL == "" {
			return "", errors.New("GeckoLink gateway address is unavailable")
		}
		return r.baseURL, nil
	}
	baseURL, err := r.discover(ctx)
	if err != nil {
		if r.baseURL != "" {
			return r.baseURL, nil
		}
		return "", err
	}
	if baseURL != r.baseURL {
		fmt.Printf("GeckoLink gateway %s resolved to %s\n", geckoLinkGatewayMAC, baseURL)
	}
	r.baseURL = baseURL
	r.resolvedAt = time.Now()
	return r.baseURL, nil
}

func discoverGeckoLinkGateway(ctx context.Context, targetMAC string, discoveryPort, restPort int) (string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return "", fmt.Errorf("open GeckoLink discovery socket: %w", err)
	}
	defer conn.Close()

	raw, err := conn.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("configure GeckoLink discovery socket: %w", err)
	}
	var socketErr error
	if err = raw.Control(func(fd uintptr) {
		socketErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); err != nil {
		return "", fmt.Errorf("configure GeckoLink discovery broadcast: %w", err)
	}
	if socketErr != nil {
		return "", fmt.Errorf("configure GeckoLink discovery broadcast: %w", socketErr)
	}

	query, _ := json.Marshal(map[string]any{
		"protocol":  "GLDP",
		"version":   1,
		"type":      "QUERY",
		"timestamp": time.Now().Unix(),
	})
	for _, address := range localBroadcastAddresses(discoveryPort) {
		_, _ = conn.WriteToUDP(query, address)
	}

	deadline := time.Now().Add(2 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err = conn.SetReadDeadline(deadline); err != nil {
		return "", err
	}
	target, err := normalizeMAC(targetMAC)
	if err != nil {
		return "", err
	}
	buffer := make([]byte, 1024)
	for {
		length, source, readErr := conn.ReadFromUDP(buffer)
		if readErr != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				return "", fmt.Errorf("GeckoLink gateway %s was not found on the local network", targetMAC)
			}
			return "", fmt.Errorf("read GeckoLink discovery response: %w", readErr)
		}
		var response gldpResponse
		if json.Unmarshal(buffer[:length], &response) != nil || response.Protocol != "GLDP" || response.Version != 1 || response.Type != "RESPONSE" {
			continue
		}
		mac, parseErr := normalizeMAC(response.Gateway.MAC)
		if parseErr != nil || mac != target {
			continue
		}
		return fmt.Sprintf("http://%s:%d", source.IP.String(), restPort), nil
	}
}

func normalizeMAC(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 12 && !strings.ContainsAny(value, ":-.") {
		value = strings.Join([]string{value[0:2], value[2:4], value[4:6], value[6:8], value[8:10], value[10:12]}, ":")
	}
	mac, err := net.ParseMAC(value)
	if err != nil {
		return "", fmt.Errorf("invalid GeckoLink gateway MAC %q: %w", value, err)
	}
	return strings.ToUpper(mac.String()), nil
}

func localBroadcastAddresses(port int) []*net.UDPAddr {
	addresses := []*net.UDPAddr{{IP: net.IPv4bcast, Port: port}}
	seen := map[string]bool{net.IPv4bcast.String(): true}
	interfaces, _ := net.Interfaces()
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		interfaceAddresses, _ := networkInterface.Addrs()
		for _, address := range interfaceAddresses {
			ipNetwork, ok := address.(*net.IPNet)
			if !ok || ipNetwork.IP.To4() == nil || len(ipNetwork.Mask) != net.IPv4len {
				continue
			}
			ip := ipNetwork.IP.To4()
			broadcast := net.IPv4(ip[0]|^ipNetwork.Mask[0], ip[1]|^ipNetwork.Mask[1], ip[2]|^ipNetwork.Mask[2], ip[3]|^ipNetwork.Mask[3])
			if !seen[broadcast.String()] {
				seen[broadcast.String()] = true
				addresses = append(addresses, &net.UDPAddr{IP: broadcast, Port: port})
			}
		}
	}
	return addresses
}
