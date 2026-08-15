package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	e.GET("/health", func(c echo.Context) error { return c.JSON(200, map[string]string{"status": "ok"}) })
	e.GET("/api/users", func(c echo.Context) error { return c.JSON(200, map[string]any{"data": []int{}}) })
	e.POST("/api/users", func(c echo.Context) error { return c.JSON(201, map[string]any{}) })

	// Dynamic route registration — should be skipped, not guessed.
	dynamicPath := computePath()
	e.GET(dynamicPath, func(c echo.Context) error { return c.JSON(200, map[string]any{}) })

	e.Start(":8080")
}

func computePath() string { return "/whatever" }
