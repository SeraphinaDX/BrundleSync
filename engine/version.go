// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionText string
var version = strings.TrimSpace(versionText)

// Version is shared by the CLI, desktop UI, Android library and APK metadata.
func Version() string { return version }
