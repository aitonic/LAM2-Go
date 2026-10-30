package desktop

import (
	_ "embed"
	"github.com/egoist/mygo/ui"
)

// These exact SVG path assets are extracted from the user-supplied LAM
// 0.4.4 apps/desktop/src/components/icons.tsx. MyGo draws the original
// strokes directly; Unicode approximations render differently per font.
//
//go:embed assets/icons/overview.svg
var overviewSVG []byte

//go:embed assets/icons/usage.svg
var usageSVG []byte

//go:embed assets/icons/sessions.svg
var sessionsSVG []byte

//go:embed assets/icons/providers.svg
var providersSVG []byte

//go:embed assets/icons/settings.svg
var settingsSVG []byte

//go:embed assets/icons/accounts.svg
var accountsSVG []byte

//go:embed assets/icons/activity.svg
var activitySVG []byte

//go:embed assets/icons/quota.svg
var quotaSVG []byte

//go:embed assets/icons/device.svg
var deviceSVG []byte

//go:embed assets/icons/sun.svg
var sunSVG []byte

//go:embed assets/icons/moon.svg
var moonSVG []byte

//go:embed assets/icons/refresh.svg
var refreshSVG []byte

//go:embed assets/icons/sliders.svg
var slidersSVG []byte

var originalOverviewIcon = ui.MustParseSVG(overviewSVG)
var originalUsageIcon = ui.MustParseSVG(usageSVG)
var originalSessionsIcon = ui.MustParseSVG(sessionsSVG)
var originalProvidersIcon = ui.MustParseSVG(providersSVG)
var originalSettingsIcon = ui.MustParseSVG(settingsSVG)
var originalAccountsIcon = ui.MustParseSVG(accountsSVG)
var originalActivityIcon = ui.MustParseSVG(activitySVG)
var originalQuotaIcon = ui.MustParseSVG(quotaSVG)
var originalDeviceIcon = ui.MustParseSVG(deviceSVG)
var originalSunIcon = ui.MustParseSVG(sunSVG)
var originalMoonIcon = ui.MustParseSVG(moonSVG)
var originalRefreshIcon = ui.MustParseSVG(refreshSVG)
var originalSlidersIcon = ui.MustParseSVG(slidersSVG)

func originalIcon(c *ui.Context, name string, size float32, color ui.Color) {
	var icon ui.Element
	switch name {
	case "overview":
		icon = ui.Icon(c, originalOverviewIcon)
	case "usage":
		icon = ui.Icon(c, originalUsageIcon)
	case "sessions":
		icon = ui.Icon(c, originalSessionsIcon)
	case "providers":
		icon = ui.Icon(c, originalProvidersIcon)
	case "settings":
		icon = ui.Icon(c, originalSettingsIcon)
	case "accounts":
		icon = ui.Icon(c, originalAccountsIcon)
	case "activity":
		icon = ui.Icon(c, originalActivityIcon)
	case "quota":
		icon = ui.Icon(c, originalQuotaIcon)
	case "device":
		icon = ui.Icon(c, originalDeviceIcon)
	case "sun":
		icon = ui.Icon(c, originalSunIcon)
	case "moon":
		icon = ui.Icon(c, originalMoonIcon)
	case "refresh":
		icon = ui.Icon(c, originalRefreshIcon)
	case "sliders":
		icon = ui.Icon(c, originalSlidersIcon)
	default:
		icon = ui.Icon(c, originalOverviewIcon)
	}
	icon.Size(size, size).TextColor(color)
}
