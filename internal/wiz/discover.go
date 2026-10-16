package wiz

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"time"
)

// DefaultScanWindow is how long Discover listens for replies.
const DefaultScanWindow = 2500 * time.Millisecond

// Subnets wider than this are skipped: probing every host would take too long.
const maxScanPrefix = 22

// Discover finds every WiZ light on the local IPv4 networks. It probes each
// host address directly because many routers drop broadcast UDP, then
// describes each light that answered.
func (c Client) Discover(ctx context.Context, window time.Duration) ([]Bulb, error) {
	targets, err := scanTargets()
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, fmt.Errorf("discover: open socket: %w", err)
	}
	defer conn.Close()
	probe := []byte(`{"method":"getPilot","params":{}}`)
	var lastSendErr error
	sent := 0
	for _, target := range targets {
		// One unreachable host must not stop the sweep; only a total failure is an error.
		if _, err := conn.WriteToUDP(probe, &net.UDPAddr{IP: target, Port: Port}); err != nil {
			lastSendErr = err
			continue
		}
		sent++
	}
	if sent == 0 {
		return nil, fmt.Errorf("discover: could not send any probe: %w", lastSendErr)
	}
	found := map[string]string{} // mac -> ip
	deadline := time.Now().Add(window)
	buf := make([]byte, maxReplyBytes)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			return nil, fmt.Errorf("discover: set deadline: %w", err)
		}
		n, from, err := conn.ReadFromUDP(buf)
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("discover: read: %w", err)
		}
		var resp struct {
			Result struct {
				MAC string `json:"mac"`
			} `json:"result"`
		}
		if json.Unmarshal(buf[:n], &resp) == nil && resp.Result.MAC != "" {
			found[resp.Result.MAC] = from.IP.String()
		}
	}
	bulbs := make([]Bulb, 0, len(found))
	for mac, ip := range found {
		bulb, err := c.Describe(ctx, ip)
		if err != nil {
			bulb = Bulb{IP: ip, MAC: mac, MinTemp: DefaultMinTemp, MaxTemp: DefaultMaxTemp, MinDim: MinDimming}
		}
		bulbs = append(bulbs, bulb)
	}
	sort.Slice(bulbs, func(i, j int) bool { return bulbs[i].MAC < bulbs[j].MAC })
	return bulbs, nil
}

// scanTargets lists every host address plus the broadcast address of each
// local IPv4 network that is up and not loopback.
func scanTargets() ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("discover: list interfaces: %w", err)
	}
	var targets []net.IP
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue // an interface that vanished mid-listing has nothing to scan
		}
		for _, addr := range addrs {
			network, ok := addr.(*net.IPNet)
			if !ok || network.IP.To4() == nil {
				continue
			}
			ones, bits := network.Mask.Size()
			if ones < maxScanPrefix || bits != 32 {
				continue
			}
			base := binary.BigEndian.Uint32(network.IP.To4().Mask(network.Mask))
			hosts := uint32(1) << (bits - ones)
			for offset := uint32(1); offset < hosts; offset++ {
				ip := make(net.IP, 4)
				binary.BigEndian.PutUint32(ip, base+offset)
				targets = append(targets, ip)
			}
		}
	}
	if len(targets) == 0 {
		return nil, errors.New("discover: no local IPv4 network to scan (is Wi-Fi connected?)")
	}
	return targets, nil
}
