package logging

import (
	"go.uber.org/zap"
)

var (
	// L is the default logger of the application
	L *zap.Logger
)

func init() {
	L, _ = zap.NewProduction(zap.WithCaller(false))
}
