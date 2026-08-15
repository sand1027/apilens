package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	api := app.Group("/api")
	users := api.Group("/users")

	users.Get("", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"data": []int{}}) })
	users.Get("/:id", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{}) })
	users.Post("", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{}) })

	app.Get("/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })

	app.Listen(":8080")
}
