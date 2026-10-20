package validate

import (
	"fmt"
	"regexp"
)

var (
	alphanumericSpaceRe = regexp.MustCompile(`^[A-Za-z0-9 ]*$`)
	alphanumericRe      = regexp.MustCompile(`^[A-Za-z0-9]*$`)
)

// AlphanumericSpace rejects every character outside A-Z, a-z, 0-9 and space,
// returning a translatable *AppValidationError. An empty value passes —
// combine with a required rule when the field is mandatory.
func AlphanumericSpace(field, value string) error {
	if !alphanumericSpaceRe.MatchString(value) {
		return NewFieldError(field, "alphanumeric_space",
			fmt.Errorf("%s: contains characters other than letters, numbers and spaces", field))
	}
	return nil
}

// Alphanumeric is AlphanumericSpace without the space allowance: only
// A-Z, a-z and 0-9 pass. An empty value passes.
func Alphanumeric(field, value string) error {
	if !alphanumericRe.MatchString(value) {
		return NewFieldError(field, "alphanumeric",
			fmt.Errorf("%s: contains characters other than letters and numbers", field))
	}
	return nil
}
