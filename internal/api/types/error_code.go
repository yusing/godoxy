package apitypes

type ErrorCode int

const (
	ErrorCodeUnauthorized ErrorCode = iota + 1
	ErrorCodeNotFound
	ErrorCodeInternalServerError
)

func (e ErrorCode) String() string {
	return []string{
		"Unauthorized",
		"Not Found",
		"Internal Server Error",
	}[e]
}
