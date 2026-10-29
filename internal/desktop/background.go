package desktop

import (
	_ "embed"
	"github.com/egoist/mygo/ui"
)

// A radial SVG native background replaces a linear approximation. The
// center and edge colors are copied from LAM 0.4.4 :root --bg CSS tokens.
// Both SVGs are decoded once and drawn by MyGo, never with a WebView.
//
//go:embed assets/bg/light.svg
var lightCanvasSVG []byte

//go:embed assets/bg/dark.svg
var darkCanvasSVG []byte

var originalLightCanvas = ui.MustParseSVG(lightCanvasSVG)
var originalDarkCanvas = ui.MustParseSVG(darkCanvasSVG)

func originalCanvasGradient(dark bool) ui.LinearGradient {
	if dark {
		return ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: ui.Hex("#161a24"), At: 0},
			{Color: ui.Hex("#0b0d12"), At: 1},
		}}
	}
	return ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: ui.Hex("#ffffff"), At: 0},
		{Color: ui.Hex("#eef0f4"), At: 1},
	}}
}

func originalCanvas(c *ui.Context, dark bool) {
	// Canvas background gradient is applied directly to the root element.
	// We deliberately avoid creating a full-screen absolute image overlay
	// that sits above sibling elements in MyGo and intercepts pointer events.
}
