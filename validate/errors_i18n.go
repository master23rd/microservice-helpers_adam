package validate

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// collectFieldErrors normalizes validator.ValidationErrors (from ShouldBind /
// ValidateStruct) and *AppValidationError (business rules, index-prefixed
// item validation) into one []FieldError shape. Validator namespace paths
// are preserved (nested struct + slice indices), not just the leaf Field().
func collectFieldErrors(err error) []FieldError {
	if err == nil {
		return nil
	}
	var appErr *AppValidationError
	if errors.As(err, &appErr) {
		return appErr.Errors
	}
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		out := make([]FieldError, 0, len(verrs))
		for _, fe := range verrs {
			out = append(out, FieldError{
				Field: namespaceToPath(fe),
				Tag:   fe.Tag(),
				Param: fe.Param(),
			})
		}
		return out
	}
	return nil
}

// namespaceToPath turns fe.Namespace() ("Sample.Series[0].MinimumOrder")
// into a dotted path ("series[0].minimum_order"), dropping the top-level
// struct name and snake_casing each segment. Indices are preserved.
func namespaceToPath(fe validator.FieldError) string {
	ns := fe.StructNamespace()
	if ns == "" {
		ns = fe.Namespace()
	}
	segs := strings.Split(ns, ".")
	if len(segs) > 1 {
		segs = segs[1:] // drop top-level struct name
	}
	var b strings.Builder
	for i, s := range segs {
		name, indices := splitIndices(s)
		if i > 0 {
			b.WriteString(".")
		}
		b.WriteString(toSnakeCase(name))
		b.WriteString(indices)
	}
	if b.Len() == 0 {
		return toSnakeCase(fe.Field())
	}
	return b.String()
}

// splitIndices splits "Series[0][1]" into ("Series", "[0][1]").
func splitIndices(seg string) (string, string) {
	if i := strings.IndexByte(seg, '['); i >= 0 {
		return seg[:i], seg[i:]
	}
	return seg, ""
}

func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := rune(s[i-1])
				if prev != '_' && (prev >= 'a' && prev <= 'z' || prev >= '0' && prev <= '9') {
					b.WriteByte('_')
				}
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// splitFieldPath turns "allocations[0].fixed" into ["allocations","0","fixed"].
func splitFieldPath(field string) []string {
	field = strings.ReplaceAll(field, "[", ".")
	field = strings.ReplaceAll(field, "]", "")
	parts := strings.Split(field, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func setNested(root map[string]any, path []string, value string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		root[path[0]] = value
		return
	}
	next, ok := root[path[0]].(map[string]any)
	if !ok {
		next = map[string]any{}
		root[path[0]] = next
	}
	setNested(next, path[1:], value)
}

// leafOf returns the last non-empty path segment, or "" when the path is empty.
func leafOf(field string) string {
	path := splitFieldPath(field)
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

// CollectFieldErrors exposes the internal field-error normalization so
// callers (e.g. web.ValidationErrorResponseWithRef) can log field/rule/param
// detail without duplicating the validator.ValidationErrors / AppValidationError
// type-switch. Returns nil for nil, unrecognized, or non-field-level errors.
func CollectFieldErrors(err error) []FieldError {
	return collectFieldErrors(err)
}

// FormatValidationErrorData builds the nested {field: message} map for the
// 422 response "data" key, translated to lang ("en"/"id"). Unknown or
// non-validation errors yield an empty map — raw details never leak.
func FormatValidationErrorData(err error, lang string) map[string]any {
	root := map[string]any{}
	for _, fe := range collectFieldErrors(err) {
		path := splitFieldPath(fe.Field)
		if len(path) == 0 {
			continue
		}
		setNested(root, path, Translate(fe.Tag, lang, path[len(path)-1], fe.Param))
	}
	return root
}

// FormatValidationErrorDataWithRef behaves like FormatValidationErrorData
// but resolves validator field names through ref's json/form tags (embedded
// structs flatten). *AppValidationError paths pass through untouched —
// they already carry final json-style paths.
func FormatValidationErrorDataWithRef(err error, lang string, ref any) map[string]any {
	if err == nil {
		return map[string]any{}
	}
	var appErr *AppValidationError
	if errors.As(err, &appErr) {
		return FormatValidationErrorData(err, lang)
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return map[string]any{}
	}
	root := map[string]any{}
	refType := typeOf(ref)
	for _, fe := range verrs {
		segments := resolveNamespace(fe, refType)
		if len(segments) == 0 {
			continue
		}
		path := expandIndices(segments)
		setNested(root, path, Translate(fe.Tag(), lang, path[len(path)-1], fe.Param()))
	}
	return root
}

// PrefixFieldErrors rewraps err (validator.ValidationErrors or
// *AppValidationError) with prefix prepended to every field path, e.g.
// prefix "allocations[0]" + field "fixed" -> "allocations[0].fixed".
// The original error stays reachable via Unwrap (errors.Is keeps working).
func PrefixFieldErrors(err error, prefix string) *AppValidationError {
	if err == nil {
		return nil
	}
	fields := collectFieldErrors(err)
	if len(fields) == 0 {
		return nil
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	out := make([]FieldError, 0, len(fields))
	for _, fe := range fields {
		f := fe.Field
		if prefix != "" {
			f = prefix + "." + strings.Trim(f, ".")
		}
		out = append(out, FieldError{Field: f, Tag: fe.Tag, Param: fe.Param})
	}
	return &AppValidationError{Errors: out, Cause: err}
}

// FormatValidationSummary builds the top-level one-line "message": the
// translated first-field message (field name already inside the template,
// never repeated). Unknown errors give the localized generic fallback.
func FormatValidationSummary(err error, lang string) string {
	fields := collectFieldErrors(err)
	if len(fields) == 0 {
		return invalidInputMessage(lang)
	}
	first := fields[0]
	return Translate(first.Tag, lang, leafOf(first.Field), first.Param)
}

func typeOf(v any) reflect.Type {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface) {
		t = t.Elem()
	}
	return t
}

// resolveNamespace maps one validator error to json-style path segments
// using refType's json/form tags; falls back to snake_cased Go names when
// refType is nil or a segment cannot be resolved.
func resolveNamespace(fe validator.FieldError, refType reflect.Type) []string {
	ns := fe.StructNamespace()
	if ns == "" {
		ns = fe.Namespace()
	}
	segs := strings.Split(ns, ".")
	if len(segs) > 1 {
		segs = segs[1:]
	}
	if refType == nil {
		out := make([]string, 0, len(segs))
		for _, s := range segs {
			name, indices := splitIndices(s)
			out = append(out, toSnakeCase(name)+indices)
		}
		if len(out) == 0 {
			return []string{toSnakeCase(fe.Field())}
		}
		return out
	}
	cur := refType
	var out []string
	for _, s := range segs {
		name, indices := splitIndices(s)
		jsonName, next, skip := resolveSegment(cur, name)
		if !skip {
			out = append(out, jsonName+indices)
		}
		cur = next
		for cur != nil && (cur.Kind() == reflect.Pointer || cur.Kind() == reflect.Interface) {
			cur = cur.Elem()
		}
		if cur != nil && (cur.Kind() == reflect.Slice || cur.Kind() == reflect.Array) {
			cur = cur.Elem()
			for cur != nil && (cur.Kind() == reflect.Pointer || cur.Kind() == reflect.Interface) {
				cur = cur.Elem()
			}
		}
	}
	return out
}

// expandIndices turns ["series[0]", "minimum_order"] into
// ["series", "0", "minimum_order"] for setNested.
func expandIndices(segments []string) []string {
	var path []string
	for _, s := range segments {
		name, indices := splitIndices(s)
		if name != "" {
			path = append(path, name)
		}
		rest := indices
		for len(rest) > 0 {
			rest = strings.TrimPrefix(rest, "[")
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				break
			}
			if num := rest[:end]; num != "" {
				path = append(path, num)
			}
			rest = rest[end+1:]
		}
	}
	return path
}

// resolveSegment finds Go field name in t (descending into anonymous
// embedded structs) and returns its json/form tag name plus field type.
// skip=true means the segment is an untagged embedded struct whose name
// mirrors json flattening (the field itself is inlined, not a path level).
func resolveSegment(t reflect.Type, goName string) (name string, next reflect.Type, skip bool) {
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface) {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return toSnakeCase(goName), nil, false
	}
	if f, ok := t.FieldByName(goName); ok {
		return tagName(f, goName), f.Type, f.Anonymous && !hasExplicitTag(f)
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		ft := f.Type
		for ft != nil && (ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Interface) {
			ft = ft.Elem()
		}
		if ft == nil || ft.Kind() != reflect.Struct {
			continue
		}
		if n, nx, sk := resolveSegment(ft, goName); nx != nil {
			return n, nx, sk
		}
	}
	return toSnakeCase(goName), nil, false
}

func hasExplicitTag(f reflect.StructField) bool {
	for _, key := range []string{"json", "form"} {
		if tag, ok := f.Tag.Lookup(key); ok {
			if name := strings.TrimSpace(strings.Split(tag, ",")[0]); name != "" && name != "-" {
				return true
			}
		}
	}
	return false
}

func tagName(f reflect.StructField, goName string) string {
	for _, key := range []string{"json", "form"} {
		if tag, ok := f.Tag.Lookup(key); ok {
			name := strings.TrimSpace(strings.Split(tag, ",")[0])
			if name != "" && name != "-" {
				return name
			}
		}
	}
	return toSnakeCase(goName)
}
