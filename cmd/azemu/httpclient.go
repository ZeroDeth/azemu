package main

import "crypto/tls"

// tlsInsecureConfig returns a TLS config that skips certificate verification.
// Used by CLI subcommands that talk to the local azemu server over its
// self-signed cert.
func tlsInsecureConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // local self-signed cert
}
