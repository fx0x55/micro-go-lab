package middleware

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
)

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func OkJson(w http.ResponseWriter, data any) {
	httpx.OkJson(w, Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

func CreatedJson(w http.ResponseWriter, data any) {
	httpx.WriteJson(w, http.StatusCreated, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

func ErrorJson(w http.ResponseWriter, status int, msg string) {
	httpx.WriteJson(w, status, Response{
		Code:    -1,
		Message: msg,
	})
}

func BadRequest(w http.ResponseWriter, msg string) {
	ErrorJson(w, http.StatusBadRequest, msg)
}

func Unauthorized(w http.ResponseWriter, msg string) {
	ErrorJson(w, http.StatusUnauthorized, msg)
}

func NotFound(w http.ResponseWriter, msg string) {
	ErrorJson(w, http.StatusNotFound, msg)
}

func InternalError(w http.ResponseWriter, msg string) {
	ErrorJson(w, http.StatusInternalServerError, msg)
}
