package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/users", func(c *gin.Context) { c.JSON(200, gin.H{"data": []int{}}) })
	r.POST("/api/users", func(c *gin.Context) { c.JSON(201, gin.H{}) })

	// Dynamic route registration — should be skipped, not guessed.
	dynamicPath := computePath()
	r.GET(dynamicPath, func(c *gin.Context) { c.JSON(200, gin.H{}) })

	r.Run(":8080")
}

func computePath() string { return "/whatever" }
