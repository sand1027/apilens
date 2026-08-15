package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	api := r.Group("/api")
	users := api.Group("/users")

	users.GET("", func(c *gin.Context) { c.JSON(200, gin.H{"data": []int{}}) })
	users.GET("/:id", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	users.POST("", func(c *gin.Context) { c.JSON(201, gin.H{}) })

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	r.Run(":8080")
}
