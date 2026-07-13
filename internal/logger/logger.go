package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"kubetask.io/kubetask/internal/config"
)

type Logger struct {
	Zap  *zap.Logger
	Atom zap.AtomicLevel
}

func New(cfg *config.Config) *Logger {
	atom := zap.NewAtomicLevel()

	level, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zapcore.InfoLevel
	}
	atom.SetLevel(level)

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder

	var encoder zapcore.Encoder
	if cfg.LogFormat == "json" {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), atom)

	zapLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return &Logger{
		Zap:  zapLogger,
		Atom: atom,
	}
}

func (l *Logger) SetLevel(level string) error {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return err
	}
	l.Atom.SetLevel(lvl)
	l.Zap.Info("Log level changed", zap.String("level", level))
	return nil
}

func (l *Logger) Sync() {
	l.Zap.Sync()
}
