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
