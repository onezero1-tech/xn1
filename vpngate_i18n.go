//go:build !windows

package main

// tr is a no-op outside Windows: the native GUI is the only place that
// translates UI strings, and vpngate.go only borrows it for node-class labels.
func tr(s string) string {
	return s
}

// shortVersion reports this build's tag for the User-Agent. Off-Windows the
// GUI card that calls the translated "开发版" fallback does not exist, so the
// stub returns the raw version string instead.
func shortVersion() string {
	if v := resolveVersion(); v != "dev" {
		return "v" + v
	}
	return "dev"
}
