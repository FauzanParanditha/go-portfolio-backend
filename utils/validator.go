package utils

import "github.com/go-playground/validator/v10"

func ValidationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "min":
		return "Minimum length is " + fe.Param()
	case "email":
		return "Invalid email format"
	default:
		return "Invalid value"
	}
}
