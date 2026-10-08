package network

import "testing"

func TestCIDRToNetmask(t *testing.T) {
	cases := map[string]string{
		"24": "255.255.255.0", "20": "255.255.240.0", "32": "255.255.255.255",
		"0": "0.0.0.0", "abc": "255.255.255.0", "+16": "255.255.0.0",
	}
	for in, want := range cases {
		if got := cidrToNetmask(in); got != want {
			t.Errorf("cidrToNetmask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsDHCPEnabled(t *testing.T) {
	config := "USE_DHCP[0]=\"no\"\nUSE_DHCP[1]=\"yes\"\n"
	if isDHCPEnabled(config, "eth0") {
		t.Error("eth0 should be static")
	}
	if !isDHCPEnabled(config, "eth1") {
		t.Error("eth1 should use DHCP")
	}
	if isDHCPEnabled(config, "wlan0") {
		t.Error("non-eth interfaces use index 0")
	}
	if !isDHCPEnabled("", "eth0") {
		t.Error("default should be DHCP")
	}
}

func TestGetConfigValue(t *testing.T) {
	if v, ok := getConfigValue("IPADDR[0]=x\nGATEWAY=\"10.0.0.1\"\n", "GATEWAY"); !ok || v != "10.0.0.1" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := getConfigValue("GATEWAY\n", "GATEWAY"); ok {
		t.Fatal("a line without '=' must not match")
	}
}
