package desktop

import (
	_ "embed"
	"github.com/egoist/mygo/ui"
)

// Same bitmap as apps/desktop/src/assets/lam-orbit-icon.png in LAM 0.4.4.
// Do not substitute a textual glyph or a new brand mark.
//
//go:embed assets/lam-orbit-icon.png
var lamOrbitPNG []byte

var lamOrbit, _ = ui.DecodeBitmap(lamOrbitPNG)

func originalLAMLogo(c *ui.Context, size float32) {
	if lamOrbit != nil {
		ui.Image(c, lamOrbit).Size(size, size).Fit(ui.Contain)
	}
}
