package main

import (
	"strings"
	"testing"
)

func TestCheckLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:8080": true, "localhost:9": true, "[::1]:8080": true, "127.0.0.2:1": true,
		"0.0.0.0:8080": false, ":8080": false, "192.168.1.5:80": false, "example.com:80": false, "8080": false,
	} {
		if err := checkLoopback(addr); (err == nil) != ok {
			t.Errorf("checkLoopback(%q) = %v, want ok=%v", addr, err, ok)
		}
	}
}

func TestDefaultAddrAndUsage(t *testing.T) {
	if defaultAddr != "127.0.0.1:7357" {
		t.Errorf("defaultAddr = %q", defaultAddr)
	}
	if got, err := resolveAddr(defaultAddr, -1); err != nil || got != "127.0.0.1:7357" {
		t.Errorf("default address does not resolve: %q %v", got, err)
	}
	if got, err := resolveAddr(defaultAddr, 7400); err != nil || got != "127.0.0.1:7400" {
		t.Errorf("-port over the default: %q %v", got, err)
	}
	for _, want := range []string{"7357", "Every command works with no arguments", "serve ", "backup ", "seed "} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
	if strings.Contains(usage, "8080") {
		t.Error("usage still mentions 8080")
	}
}

func TestResolveAddr(t *testing.T) {
	for _, c := range []struct {
		addr string
		port int
		want string // empty: an error
	}{
		{"127.0.0.1:8080", -1, "127.0.0.1:8080"},
		{"127.0.0.1:8080", 8081, "127.0.0.1:8081"},
		{"localhost:8080", 9000, "localhost:9000"},
		{"[::1]:8080", 9000, "[::1]:9000"},
		{"127.0.0.1:8080", 65535, "127.0.0.1:65535"},
		{"127.0.0.1:8080", 0, ""},
		{"127.0.0.1:8080", 65536, ""},
		{"127.0.0.1:8080", -2, ""},
		{"127.0.0.1:0", -1, ""},
		{"127.0.0.1:http", -1, ""},
		{"0.0.0.0:8080", 8081, ""}, // -port does not lift the loopback rule
		{":8080", -1, ""},
		{"8080", 8081, ""},
	} {
		got, err := resolveAddr(c.addr, c.port)
		if (err == nil) != (c.want != "") || got != c.want && c.want != "" {
			t.Errorf("resolveAddr(%q, %d) = %q, %v; want %q", c.addr, c.port, got, err, c.want)
		}
	}
}

func TestRunBackupUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"a", "b"}} {
		if err := runBackup(args); err == nil {
			t.Errorf("runBackup(%q) accepted wrong arguments", args)
		}
	}
}
