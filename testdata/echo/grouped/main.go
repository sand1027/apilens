package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	api := e.Group("/api")
	users := api.Group("/users")

	users.GET("", func(c echo.Context) error { return c.JSON(200, map[string]any{"data": []int{}}) })
	users.GET("/:id", func(c echo.Context) error { return c.JSON(200, map[string]any{}) })
	users.POST("", func(c echo.Context) error { return c.JSON(201, map[string]any{}) })

	e.GET("/health", func(c echo.Context) error { return c.JSON(200, map[string]string{"status": "ok"}) })

	e.Start(":8080")
}
