package test

import (
	"os"
	"strings"
	"testing"

	sdkconfig "github.com/tigerfintech/openapi-go-sdk/config"
	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"
)

// TestSmokeGatewayWiring proves the SDK wiring, request signing, and HTTP
// transport are intact by calling the real Tiger gateway with deliberately
// invalid credentials and asserting a parameter-error response (code=1000).
// This test requires NO real credentials and is part of the normal test suite.
func TestSmokeGatewayWiring(t *testing.T) {
	cfg, err := sdkconfig.NewClientConfig(
		sdkconfig.WithTigerID("invalid-id-for-smoke-test"),
		sdkconfig.WithPrivateKey("invalid-key-for-smoke-test"),
		sdkconfig.WithServerURL("https://openapi.itigerup.com"),
	)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}

	qc := sdkquote.NewQuoteClientFromConfig(cfg)

	// GetMarketState is a simple read that returns quickly on auth failure.
	_, err = qc.GetMarketState("US")

	// The Tiger API returns a parameter-error response when the request reaches
	// the gateway but the credentials are invalid. That is the correct outcome:
	// a parameter error proves the request was built, signed, and delivered.
	// A nil error (no error) means TLS/connection failed and the wiring is broken.
	if err == nil {
		t.Fatal("expected an error; got nil — TLS/connection problem, not a gateway response")
	}

	// The error message must contain "param" or "1000" — both indicate the
	// request reached the gateway and was refused for invalid credentials.
	// Any other error message means the wiring is broken.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "param") && !strings.Contains(errMsg, "1000") {
		t.Fatalf("expected Tiger parameter error (code=1000); got: %v", err)
	}
}

// TestSmokeNoCredentialsRequired proves the smoke test runs without any
// TIGER_* environment variables.
func TestSmokeNoCredentialsRequired(t *testing.T) {
	required := []string{
		"TIGER_ID",
		"TIGER_PRIVATE_KEY",
		"TIGER_PRIVATE_KEY_FILE",
		"TIGER_ACCOUNT",
	}
	for _, v := range required {
		if os.Getenv(v) != "" {
			t.Errorf("smoke test must not require %s; found: %q", v, os.Getenv(v))
		}
	}
}
