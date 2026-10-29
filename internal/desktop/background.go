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

func originalCanvas(c *ui.Context, dark bool) {
	image := originalLightCanvas
	if dark {
		image = originalDarkCanvas
	}
	ui.Image(c, image).Absolute().Left(0).Right(0).Top(0).Bottom(0).Fit(ui.FillBox)
}
