// Package agentdoc is the usage text agents get from simplex help and plugin status.
package agentdoc

import _ "embed"

//go:embed instructions.md
var Text string
