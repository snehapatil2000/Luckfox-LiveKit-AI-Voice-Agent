package main

import "testing"

func TestNormalizeGatewayMAC(t *testing.T) {
	for _, input := range []string{"30-1f-9a-d1-45-49", "301F9AD14549"} {
		got, err := normalizeMAC(input)
		if err != nil {
			t.Fatal(err)
		}
		if got != geckoLinkGatewayMAC {
			t.Fatalf("normalizeMAC(%q) = %q, want %q", input, got, geckoLinkGatewayMAC)
		}
	}
}

func TestStaticGatewayResolver(t *testing.T) {
	resolver := staticGatewayResolver("http://127.0.0.1:8889/")
	url, err := resolver.URL(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://127.0.0.1:8889" {
		t.Fatalf("URL() = %q", url)
	}
}
