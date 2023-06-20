// Package log is a temporary package until we forge our log structure.
package log

import (
	"context"

	"github.com/sirupsen/logrus"
)

type (
	// TextFormatter is the text formatter for the logs.
	TextFormatter = logrus.TextFormatter
)

var (
	// SetFormatter sets the standard logger formatter.
	SetFormatter = logrus.SetFormatter
	// SetLevel sets the standard logger level.
	SetLevel = logrus.SetLevel
	// SetReportCaller sets whether the standard logger will include the calling method as a field.
	SetReportCaller = logrus.SetReportCaller
)

const (
	// InfoLevel level. General operational entries about what's going on inside the application.
	InfoLevel = logrus.InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel = logrus.DebugLevel
)

// Debug is a temporary placeholder.
func Debug(ctx context.Context, args ...interface{}) {
	logrus.Debug(args...)
}

// Debugf is a temporary placeholder.
func Debugf(ctx context.Context, format string, args ...interface{}) {
	logrus.Debugf(format, args...)
}

// Info is a temporary placeholder.
func Info(ctx context.Context, args ...interface{}) {
	logrus.Info(args...)
}

// Warningf is a temporary placeholder.
func Warningf(ctx context.Context, format string, args ...interface{}) {
	logrus.Warningf(format, args...)
}

// Error is a temporary placeholder.
func Error(ctx context.Context, args ...interface{}) {
	logrus.Error(args...)
}

// Infof is a temporary placeholder.
func Infof(ctx context.Context, format string, args ...interface{}) {
	logrus.Infof(format, args...)
}
