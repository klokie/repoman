package host

import (
	"os"
	"strings"
)

// Name is the short, lowercased hostname of this machine ("gatekeeper.local"
// and "Gatekeeper" both resolve to "gatekeeper"). REPOMAN_HOST overrides it,
// which is what makes dry-running another host's view possible.
func Name() string {
	if h := os.Getenv("REPOMAN_HOST"); h != "" {
		return strings.ToLower(h)
	}
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return strings.ToLower(strings.Split(h, ".")[0])
}
