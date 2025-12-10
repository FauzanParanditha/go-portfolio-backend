package validation

import (
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

func ValidateStruct(s any) error {
	return validate.Struct(s)
}

// Ubah ValidationErrors → map[field]message sederhana
func ToFieldErrors(err error) map[string]string {
	res := map[string]string{}

	if verrs, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range verrs {
			field := fe.Field()

			switch fe.Tag() {
			case "required":
				res[field] = "wajib diisi"
			case "email":
				res[field] = "format email tidak valid"
			case "url":
				res[field] = "format URL tidak valid"
			default:
				res[field] = "tidak valid"
			}
		}
	}

	return res
}
