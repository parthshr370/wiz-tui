package wiz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// The bulb recomputes the HMAC over the params bytes it receives, so the
// signature must cover exactly the sent params, sigTs included.
func TestSignedSetPilotSignsTheSentParams(t *testing.T) {
	key, err := hex.DecodeString("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	change := Color(255, 0, 0, 40)
	packet, err := Client{SigningKey: key}.encode("setPilot", &change, true)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		HMAC   string          `json:"hmac"`
	}
	if err := json.Unmarshal(packet, &wire); err != nil {
		t.Fatalf("packet is not JSON: %v", err)
	}
	var params struct {
		SigTs   int `json:"sigTs"`
		Red     int `json:"r"`
		Dimming int `json:"dimming"`
	}
	if err := json.Unmarshal(wire.Params, &params); err != nil {
		t.Fatalf("params: %v", err)
	}
	if params.SigTs == 0 || params.Red != 255 || params.Dimming != 40 {
		t.Fatalf("params missing sigTs or the change: %s", wire.Params)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(wire.Params)
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); wire.HMAC != want {
		t.Fatalf("hmac %q does not cover the sent params; want %q", wire.HMAC, want)
	}
}
