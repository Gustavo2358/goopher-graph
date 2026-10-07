package installedwasm

import "embed"

//go:generate sh ../../tools/build_installed_wasm.sh
//go:embed assets/*.wasm
var assets embed.FS
