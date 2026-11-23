package response

import "github.com/gin-gonic/gin"

// JSON 统一写 JSON 响应。
func JSON(c *gin.Context, status int, body any) {
	c.JSON(status, body)
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// Error 写统一错误体 {"error":{code,message,request_id}}。
func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, errorEnvelope{Error: errorBody{
		Code:      code,
		Message:   message,
		RequestID: c.GetString("request_id"),
	}})
}