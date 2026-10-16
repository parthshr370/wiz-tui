// Package wiz speaks the local UDP protocol of Philips WiZ lights.
//
// Every message is one JSON object sent to UDP port 38899 and answered by one
// JSON object. Reads (getPilot, getSystemConfig, getModelConfig) are always
// accepted. Writes (setPilot) on firmware 1.34+ are accepted unsigned only when
// the WiZ app allows "All controls"; otherwise they must carry an HMAC made
// with the home's udp_signing_key.
package wiz

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// Port is the UDP port every WiZ light listens on.
const Port = 38899

const (
	defaultTimeout = 1500 * time.Millisecond
	maxReplyBytes  = 4096
	codeBadParams  = -32602
)

// Client sends requests to lights. A nil SigningKey sends unsigned writes.
type Client struct {
	SigningKey []byte
	Timeout    time.Duration
}

// BulbError is an error object returned by the light itself.
type BulbError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Signed  bool   `json:"-"`
}

func (e *BulbError) Error() string {
	if e.Code == codeBadParams && !e.Signed {
		return "bulb refused the change (Invalid params). In the WiZ app open Settings > Security " +
			"(under Current Home) and set Local communication to \"All controls\"; if it already says so, " +
			"switch to \"Only verified controls\", save, and switch back. Or save the home key in ~/.config/wiz/key."
	}
	if e.Code == codeBadParams {
		return "bulb refused the signed change (Invalid params): check ~/.config/wiz/key is this home's udp_signing_key"
	}
	return fmt.Sprintf("bulb error %d: %s", e.Code, e.Message)
}

type reply struct {
	Result json.RawMessage `json:"result"`
	Error  *BulbError      `json:"error"`
}

func (c Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultTimeout
}

// encode builds the wire bytes. A nil change sends empty params (a read).
// Signed writes add sigTs to params and an HMAC over the exact compact
// params bytes that are sent.
func (c Client) encode(method string, change *Change, sign bool) ([]byte, error) {
	params := json.RawMessage(`{}`)
	if change != nil {
		raw, err := json.Marshal(change)
		if err != nil {
			return nil, fmt.Errorf("encode %s params: %w", method, err)
		}
		params = raw
	}
	if !sign {
		return json.Marshal(struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}{method, params})
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(params, &fields); err != nil {
		return nil, fmt.Errorf("re-read %s params: %w", method, err)
	}
	fields["sigTs"] = json.RawMessage(fmt.Sprint(time.Now().Unix()))
	signed, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode signed %s params: %w", method, err)
	}
	mac := hmac.New(sha256.New, c.SigningKey)
	mac.Write(signed)
	return json.Marshal(struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		HMAC   string          `json:"hmac"`
	}{method, signed, base64.StdEncoding.EncodeToString(mac.Sum(nil))})
}

// Call sends one request to ip and returns the raw "result" object.
// change is nil for reads.
func (c Client) Call(ctx context.Context, ip, method string, change *Change) (json.RawMessage, error) {
	sign := len(c.SigningKey) > 0 && change != nil
	packet, err := c.encode(method, change, sign)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp4", net.JoinHostPort(ip, fmt.Sprint(Port)))
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, ip, err)
	}
	defer conn.Close()
	deadline := time.Now().Add(c.timeout())
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("%s %s: set deadline: %w", method, ip, err)
	}
	if _, err := conn.Write(packet); err != nil {
		return nil, fmt.Errorf("%s %s: send: %w", method, ip, err)
	}
	buf := make([]byte, maxReplyBytes)
	n, err := conn.Read(buf)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("%s %s: no reply within %s (bulb off at the wall, or on another network?)", method, ip, c.timeout())
		}
		return nil, fmt.Errorf("%s %s: read: %w", method, ip, err)
	}
	var resp reply
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return nil, fmt.Errorf("%s %s: bad reply %q: %w", method, ip, buf[:n], err)
	}
	if resp.Error != nil {
		resp.Error.Signed = sign
		return nil, resp.Error
	}
	return resp.Result, nil
}

// Status reads the light's current state.
func (c Client) Status(ctx context.Context, ip string) (State, error) {
	var state State
	result, err := c.Call(ctx, ip, "getPilot", nil)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(result, &state); err != nil {
		return state, fmt.Errorf("getPilot %s: decode: %w", ip, err)
	}
	return state, nil
}

// Set applies a change.
func (c Client) Set(ctx context.Context, ip string, change Change) error {
	_, err := c.Call(ctx, ip, "setPilot", &change)
	return err
}

// Describe reads identity and capabilities for the light at ip.
func (c Client) Describe(ctx context.Context, ip string) (Bulb, error) {
	var system struct {
		MAC      string `json:"mac"`
		Module   string `json:"moduleName"`
		Firmware string `json:"fwVersion"`
	}
	result, err := c.Call(ctx, ip, "getSystemConfig", nil)
	if err != nil {
		return Bulb{}, err
	}
	if err := json.Unmarshal(result, &system); err != nil {
		return Bulb{}, fmt.Errorf("getSystemConfig %s: decode: %w", ip, err)
	}
	bulb := Bulb{IP: ip, MAC: system.MAC, Module: system.Module, Firmware: system.Firmware,
		MinTemp: DefaultMinTemp, MaxTemp: DefaultMaxTemp, MinDim: MinDimming}
	var model struct {
		CCTRange    []int `json:"cctRange"`
		MinDimLevel int   `json:"minDimLevel"`
	}
	// Older firmware has no getModelConfig; the defaults above stay in that case.
	if result, err := c.Call(ctx, ip, "getModelConfig", nil); err == nil && json.Unmarshal(result, &model) == nil {
		if len(model.CCTRange) >= 2 {
			bulb.MinTemp, bulb.MaxTemp = model.CCTRange[0], model.CCTRange[len(model.CCTRange)-1]
		}
		if model.MinDimLevel > 0 {
			bulb.MinDim = model.MinDimLevel
		}
	}
	return bulb, nil
}
