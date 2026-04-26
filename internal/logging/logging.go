/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

package logging

import (
	"log/slog"
	"os"
)

var logger *slog.Logger

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError + 1, // effectively disabled
	}))
}

// Init initializes the logger. Call with debug=true to enable debug output.
func Init(debug bool) {
	if debug {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}
}

// Logger returns the global logger instance
func Logger() *slog.Logger {
	return logger
}

// Debug logs at debug level
func Debug(msg string, args ...any) {
	logger.Debug(msg, args...)
}

// Info logs at info level
func Info(msg string, args ...any) {
	logger.Info(msg, args...)
}

// Warn logs at warn level
func Warn(msg string, args ...any) {
	logger.Warn(msg, args...)
}

// Error logs at error level
func Error(msg string, args ...any) {
	logger.Error(msg, args...)
}
