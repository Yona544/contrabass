//go:build !dashboard_dist

package contrabass

import "embed"

// DashboardDistFS is nil when the dashboard_dist build tag is absent.
// The zero value of embed.FS is usable; fs.Sub on it returns an error,
// which main.go already handles via a nil check.
var DashboardDistFS embed.FS
