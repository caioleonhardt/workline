package main

import "testing"

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
