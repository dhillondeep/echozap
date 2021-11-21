package echozap

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// DefaultCustomFieldsKey is the key for custom fields in the context.
	DefaultCustomFieldsKey = "_echozap_custom_fields_"
	// DefaultCustomLoggerKey is the key for custom logger in the context.
	DefaultCustomLoggerKey = "_echozap_custom_logger_"
)

type Options struct {
	// Logger is the zap logger to use
	Logger *zap.Logger
	// CustomFieldsKey is the key to use for custom fields (default: echozap.DefaultCustomFieldsPrefix)
	CustomFieldsKey string
	// CustomLoggerKey is the key to use for the custom logger (default: echozap.DefaultCustomLoggerKey)
	CustomLoggerKey string
}

// ZapLogger is a middleware and zap to provide an "access log" like logging for each request.
func ZapLogger(options *Options) echo.MiddlewareFunc {
	logger := options.Logger

	if options.CustomFieldsKey == "" {
		options.CustomFieldsKey = DefaultCustomFieldsKey
	}
	if options.CustomLoggerKey == "" {
		options.CustomLoggerKey = DefaultCustomLoggerKey
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			customerLogger := getLoggerFromContext(c, options.CustomLoggerKey)
			if customerLogger != nil {
				logger = customerLogger
			}

			start := time.Now()

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()

			fields := []zapcore.Field{
				zap.String("remote_ip", c.RealIP()),
				zap.String("latency", time.Since(start).String()),
				zap.String("host", req.Host),
				zap.String("request", fmt.Sprintf("%s %s", req.Method, req.RequestURI)),
				zap.Int("status", res.Status),
				zap.Int64("size", res.Size),
				zap.String("user_agent", req.UserAgent()),
			}

			// add custom fields if provided and valid
			customFields, ok := c.Get(options.CustomFieldsKey).([]zapcore.Field)
			if ok {
				fields = append(fields, customFields...)
			}

			id := req.Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = res.Header().Get(echo.HeaderXRequestID)
				fields = append(fields, zap.String("request_id", id))
			}

			n := res.Status
			text := http.StatusText(n)
			switch {
			case n >= 500:
				logger.With(zap.Error(err)).Error(fmt.Sprintf("Server: %s", text), fields...)
			case n >= 400:
				logger.With(zap.Error(err)).Warn(fmt.Sprintf("Client: %s", text), fields...)
			case n >= 300:
				logger.Info(fmt.Sprintf("Redirection: %s", text), fields...)
			default:
				logger.Info(fmt.Sprintf("Success: %s", text), fields...)
			}

			return nil
		}
	}
}

// getLoggerFromContext returns the logger from the context
func getLoggerFromContext(c echo.Context, loggerKey string) *zap.Logger {
	contextData := c.Get(loggerKey)
	var logger *zap.Logger

	customLogger, ok := contextData.(*zap.Logger)
	if ok {
		logger = customLogger
	} else {
		// try sugared logger if other one failed
		customLogger, ok := c.Get(loggerKey).(*zap.SugaredLogger)
		if ok {
			logger = customLogger.Desugar()
		}
	}

	return logger
}
