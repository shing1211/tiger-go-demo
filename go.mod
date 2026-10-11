module github.com/shing1211/tiger-go-demo

go 1.24

require (
	github.com/tigerfintech/openapi-go-sdk v0.5.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

// TIGER-GO-DEMO HARDENING NOTE:
// github.com/golang/protobuf v1.5.0 is deprecated and carries known CVEs.
// It appears in the module graph only as a transitive dep of gopkg.in/check.v1
// (test infrastructure). check.v1 cannot be upgraded without breaking API
// compatibility with the existing test suite. No production code imports it directly.
// The golang/protobuf dep will be removed when check.v1 releases a compatible fix.
exclude github.com/golang/protobuf v1.5.0
