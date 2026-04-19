package erroradvisor

import (
	"context"
	"errors"
	"net"
	"strings"

	relayerrors "relay/internal/errors"
)

// Suggest returns a concise, actionable recommendation for the given error.
func Suggest(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "Check origin server availability or increase timeout values."
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "Verify the origin URL host and DNS configuration."
	}

	if appErr, ok := relayerrors.AsAppError(err); ok {
		switch appErr.Category {
		case relayerrors.CategoryTimeout:
			return "Check origin server availability or increase timeout values."
		case relayerrors.CategoryNetwork:
			return "Verify network connectivity and origin reachability from the relay host."
		case relayerrors.CategoryCache:
			if strings.Contains(strings.ToLower(appErr.Code), "overflow") {
				return "Increase cache size limits or adjust eviction policy settings."
			}
			return "Review cache limits and bypass rules for this workload."
		case relayerrors.CategoryConfig:
			return "Check configuration file, environment variables, or CLI arguments for invalid values."
		case relayerrors.CategoryRate:
			return "Reduce request rate or increase rate limit thresholds."
		default:
			return "Inspect relay logs and origin health to identify root cause."
		}
	}

	return "Inspect relay logs and origin health to identify root cause."
}
