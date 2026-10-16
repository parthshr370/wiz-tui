package config

import (
	"context"
	"fmt"
	"time"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const knownProbeTimeout = 700 * time.Millisecond

// KnownBulbs describes every remembered bulb that still answers at its saved
// IP with the same MAC. Bulbs that moved or are powered off are left out.
func KnownBulbs(ctx context.Context, client wiz.Client, store Store) []wiz.Bulb {
	quick := client
	quick.Timeout = knownProbeTimeout
	var bulbs []wiz.Bulb
	for mac, known := range store.Bulbs {
		bulb, err := quick.Describe(ctx, known.IP)
		if err == nil && bulb.MAC == mac {
			bulbs = append(bulbs, bulb)
		}
	}
	return bulbs
}

// Rediscover scans the network, remembers every bulb found and saves.
func Rediscover(ctx context.Context, client wiz.Client, store *Store) ([]wiz.Bulb, error) {
	bulbs, err := client.Discover(ctx, wiz.DefaultScanWindow)
	if err != nil {
		return nil, err
	}
	for _, bulb := range bulbs {
		store.Remember(bulb.MAC, bulb.IP)
	}
	if err := store.Save(); err != nil {
		return bulbs, err
	}
	return bulbs, nil
}

// Find returns the bulb a query names (MAC, IP or name; empty = default).
// It tries the saved IP first and rescans the network only when needed.
func Find(ctx context.Context, client wiz.Client, store *Store, query string) (wiz.Bulb, error) {
	if mac, ok := store.Resolve(query); ok {
		quick := client
		quick.Timeout = knownProbeTimeout
		if bulb, err := quick.Describe(ctx, store.Bulbs[mac].IP); err == nil && bulb.MAC == mac {
			return bulb, nil
		}
	}
	bulbs, err := Rediscover(ctx, client, store)
	if err != nil {
		return wiz.Bulb{}, err
	}
	if len(bulbs) == 0 {
		return wiz.Bulb{}, fmt.Errorf("no WiZ light answered on this network (is it powered, and is this Mac on the same Wi-Fi?)")
	}
	mac, ok := store.Resolve(query)
	if !ok {
		if query != "" {
			return wiz.Bulb{}, fmt.Errorf("no bulb matches %q; run `wiz list` to see names and IPs", query)
		}
		mac = bulbs[0].MAC
	}
	for _, bulb := range bulbs {
		if bulb.MAC == mac {
			return bulb, nil
		}
	}
	return wiz.Bulb{}, fmt.Errorf("bulb %q is remembered but did not answer the scan (powered off at the wall?)", query)
}
