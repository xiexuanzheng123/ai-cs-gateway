package chat

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func writeError(c *gin.Context, status int, code string, err error) {
	message := code
	if err != nil {
		message = err.Error()
	}
	c.JSON(status, gin.H{
		"code":  code,
		"error": message,
	})
}

func writeBadRequest(c *gin.Context, code string, err error) {
	writeError(c, http.StatusBadRequest, code, err)
}

func writeUpstreamError(c *gin.Context, code string, err error) {
	writeError(c, http.StatusBadGateway, code, err)
}
