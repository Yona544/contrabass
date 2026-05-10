//go:build dashboard_dist

package contrabass

import "embed"

// DashboardDistFS embeds the built dashboard assets.
// This file is only compiled when the dashboard_dist build tag is present
// (set by the Makefile during full builds).  When the tag is absent
// (e.g. go install without JS build), the variable is nil and the
// dashboard is not served.
//
//go:embed all:packages/dashboard/dist
var DashboardDistFS embed.FS
