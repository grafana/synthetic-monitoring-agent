//go:build tools
// +build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/quasilyte/go-ruleguard/dsl"
	_ "github.com/securego/gosec/cmd/gosec"
	_ "github.com/unknwon/bra"
	_ "gotest.tools/gotestsum"
)
