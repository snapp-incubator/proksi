package logging

import (
	"go.uber.org/zap"
)

var (
	L *zap.Logger
)

func init() {
	L, _ = zap.NewProduction(zap.WithCaller(false))
}
