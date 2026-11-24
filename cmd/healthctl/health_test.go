package main

import "testing"

func TestHealthURLFollowsListenAddress(t *testing.T) {
	for listen, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/readyz",
		"127.0.0.1:8437": "http://127.0.0.1:8437/readyz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/readyz",
		"[::]:9001":      "http://127.0.0.1:9001/readyz",
		"[::1]:9002":     "http://[::1]:9002/readyz",
	} {
		if got := healthURL(listen); got != want {
			t.Fatalf("%s: got %s, want %s", listen, got, want)
		}
	}
}
