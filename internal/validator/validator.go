package validator

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/zeromicro/go-zero/rest/httpx"
)

type goValidator struct {
	v *validator.Validate
}

func (gv *goValidator) Validate(_ *http.Request, data any) error {
	return gv.v.Struct(data)
}

func Init() {
	httpx.SetValidator(&goValidator{v: validator.New()})
}
