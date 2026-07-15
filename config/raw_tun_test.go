package config

import (
	"encoding/json"
	"testing"
)

func TestRawTunJSONUsesMihomoAutoDetectInterfaceKey(t *testing.T) {
	data, err := json.Marshal(RawTun{AutoDetectInterface: true})
	if err != nil {
		t.Fatalf("marshal RawTun: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal RawTun JSON: %v", err)
	}

	if got := fields["auto-detect-interface"]; got != true {
		t.Fatalf("auto-detect-interface = %v, want true", got)
	}
	if _, found := fields["AutoDetectInterface"]; found {
		t.Fatal("unexpected Go field name in RawTun JSON")
	}
}
