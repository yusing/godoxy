package apitypes

type SuccessResponse struct {
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty" extensions:"x-nullable"`
} // @name SuccessResponse

func Success(message string, extra ...map[string]any) SuccessResponse {
	if len(extra) > 0 {
		return SuccessResponse{
			Message: message,
			Details: extra[0],
		}
	}
	return SuccessResponse{
		Message: message,
	}
}
