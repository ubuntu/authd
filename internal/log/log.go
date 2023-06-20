// package log is a temporary package until we forge our log structure.
package log

import (
	"context"

	"github.com/sirupsen/logrus"
)

type (
	TextFormatter = logrus.TextFormatter
)

var (
	SetFormatter    = logrus.SetFormatter
	SetLevel        = logrus.SetLevel
	SetReportCaller = logrus.SetReportCaller
)

const (
	InfoLevel  = logrus.InfoLevel
	DebugLevel = logrus.DebugLevel
)

func Debug(ctx context.Context, args ...interface{}) {
	logrus.Debug(args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	logrus.Debugf(format, args...)
}

func Info(ctx context.Context, args ...interface{}) {
	logrus.Info(args...)
}

func Warningf(ctx context.Context, format string, args ...interface{}) {
	logrus.Warningf(format, args...)
}

func Error(ctx context.Context, args ...interface{}) {
	logrus.Error(args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	logrus.Infof(format, args...)
}
