package desktop

import _ "embed"

// Byte-identical macOS status-item template icon from LAM 0.4.4.
//
//go:embed assets/tray-menu-template.png
var originalTrayTemplate []byte
