package interfaces

type ValidationError struct {
	Field    string
	ErrorMsg string
}

type Validation interface {
	Struct(input any) []ValidationError
}
