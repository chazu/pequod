// Package cue provides embedded CUE platform modules.
package cue

import "embed"

// PlatformFS contains the embedded platform CUE modules.
// This embeds all .cue files from the platform directory and its subdirectories.
//
//go:embed platform/*/*.cue platform/policy/*.cue
var PlatformFS embed.FS

// PlatformDir is the root directory within the embedded filesystem.
const PlatformDir = "platform"
