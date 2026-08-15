package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	app.Get("/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	app.Get("/api/users", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"data": []int{}}) })
	app.Post("/api/users", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{}) })

	// Dynamic route registration — should be skipped, not guessed.
	dynamicPath := computePath()
	app.Get(dynamicPath, func(c *fiber.Ctx) error { return c.JSON(fiber.Map{}) })

	app.Listen(":8080")
}

func computePath() string { return "/whatever" }
