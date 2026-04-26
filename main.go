/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

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
