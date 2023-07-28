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
func Debug(_ context.Context, args ...interface{}) {
	logrus.Debug(args...)
}

// Debugf is a temporary placeholder.
func Debugf(_ context.Context, format string, args ...interface{}) {
	logrus.Debugf(format, args...)
}

// Info is a temporary placeholder.
func Info(_ context.Context, args ...interface{}) {
	logrus.Info(args...)
}

// Warningf is a temporary placeholder.
func Warningf(_ context.Context, format string, args ...interface{}) {
	logrus.Warningf(format, args...)
}

// Error is a temporary placeholder.
func Error(_ context.Context, args ...interface{}) {
	logrus.Error(args...)
}

// Errorf is a temporary placeholder.
func Errorf(_ context.Context, format string, args ...interface{}) {
	logrus.Errorf(format, args...)
}

// Infof is a temporary placeholder.
func Infof(_ context.Context, format string, args ...interface{}) {
	logrus.Infof(format, args...)
}
