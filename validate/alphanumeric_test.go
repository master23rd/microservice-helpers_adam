package validate

import (
	"errors"
	"testing"
)

func TestAlphanumericSpace(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string passes", "", false},
		{"letters only", "HelloWorld", false},
		{"digits only", "12345", false},
		{"letters digits and spaces", "Room 101 B", false},
		{"leading and trailing space", " ok ", false},
		{"rejects punctuation", "hello!", true},
		{"rejects symbol", "50%", true},
		{"rejects unicode letter", "café", true},
		{"rejects newline", "line1\nline2", true},
		{"rejects tab", "a\tb", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AlphanumericSpace("name", tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AlphanumericSpace(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAlphanumeric(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string passes", "", false},
		{"letters only", "HelloWorld", false},
		{"digits only", "12345", false},
		{"letters and digits", "Room101B", false},
		{"rejects space", "Room 101", true},
		{"rejects punctuation", "hello!", true},
		{"rejects unicode letter", "café", true},
		{"rejects newline", "a\nb", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Alphanumeric("code", tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Alphanumeric(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// Failures must carry the field path and a message key, so the 422 response
// builder can translate them like any other field error.
func TestAlphanumericErrorsAreTranslatableFieldErrors(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		field   string
		wantTag string
	}{
		{"with space", AlphanumericSpace("full_name", "bad!"), "full_name", "alphanumeric_space"},
		{"strict", Alphanumeric("code", "bad!"), "code", "alphanumeric"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var appErr *AppValidationError
			if !errors.As(tc.err, &appErr) {
				t.Fatalf("got %T, want *AppValidationError", tc.err)
			}
			if len(appErr.Errors) != 1 {
				t.Fatalf("got %d field errors, want 1", len(appErr.Errors))
			}
			fe := appErr.Errors[0]
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q", fe.Field, tc.field)
			}
			if fe.Tag != tc.wantTag {
				t.Errorf("tag = %q, want %q", fe.Tag, tc.wantTag)
			}

			// The catalogs must carry the key; a missing key silently degrades
			// to the generic "_error" template in both languages.
			generic := map[string]string{"en": tc.field + " is invalid", "id": tc.field + " tidak valid"}
			for _, lang := range []string{"en", "id"} {
				got := Translate(fe.Tag, lang, fe.Field, fe.Param)
				if got == generic[lang] {
					t.Errorf("tag %q missing from messages_%s.json (fell back to %q)", fe.Tag, lang, got)
				}
			}
		})
	}
}
