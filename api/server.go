package main

import (
	"api/internal/config"
	"api/internal/routes"
	"api/internal/shared"

	"github.com/aidarkhanov/nanoid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("Failed to get logger")
	}
	sugar := logger.Sugar()

	cfg, errs := config.InitConfig()
	if errs != nil {
		for _, err := range errs {
			sugar.Errorln(err)
		}
		panic("Failed to init config")
	}
	defer cfg.Shutdown()

	e := echo.New()
	e.Use(middleware.CORS())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqId, _ := nanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyz", 28)
			logger := sugar.With(
				"request_id", "req_"+reqId,
			)

			cc := &shared.Context{Context: c, Log: logger, Reqid: reqId, Cfg: cfg}
			return next(cc)
		}
	})
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		StackSize: 1 << 10, // 1 KB
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			defer func() {
				_ = sugar.Sync()
			}()
			sugar.Errorw("Api Panic", "error", err.Error())
			return c.String(500, "Internal Server Error")
		},
	}))

	// Create a group for admin endpoints
	adminGroup := e.Group("/admin")

	// Create a group for verification endpoints
	verifyGroup := e.Group("")

	// Apply admin routes
	adminGroup.POST("/add-key", routes.AddKey)
	adminGroup.POST("/remove-key", routes.RemoveKey)
	adminGroup.POST("/get-key", routes.GetKey)

	// Apply verify route
	verifyGroup.POST("/verify", routes.Verify)

	e.Logger.Fatal(e.Start(":80"))
}
