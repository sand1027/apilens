package security

import "testing"

func TestValidateListenAddr_LoopbackAllowed(t *testing.T) {
	cases := []string{"127.0.0.1:8888", "localhost:8888", "[::1]:8888"}
	for _, addr := range cases {
		if err := ValidateListenAddr(addr, false); err != nil {
			t.Errorf("ValidateListenAddr(%q, false) = %v, want nil", addr, err)
		}
	}
}

func TestValidateListenAddr_NonLoopbackDeniedWithoutFlag(t *testing.T) {
	cases := []string{"0.0.0.0:8888", "192.168.1.5:8888", ":8888"}
	for _, addr := range cases {
		if err := ValidateListenAddr(addr, false); err == nil {
			t.Errorf("ValidateListenAddr(%q, false) = nil, want error", addr)
		}
	}
}

func TestValidateListenAddr_NonLoopbackAllowedWithFlag(t *testing.T) {
	if err := ValidateListenAddr("0.0.0.0:8888", true); err != nil {
		t.Errorf("ValidateListenAddr with allowRemote=true = %v, want nil", err)
	}
}
