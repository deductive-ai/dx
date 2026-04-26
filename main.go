// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/deductive-ai/dx/cmd"
	"github.com/deductive-ai/dx/internal/telemetry"
)

func main() {
	shutdown := telemetry.Init(cmd.Version)
	defer shutdown()
	cmd.Execute()
}
