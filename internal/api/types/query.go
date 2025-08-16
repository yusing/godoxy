package apitypes

type QueryOptions struct {
	Limit   int                 `binding:"required,min=1,max=20" form:"limit"`
	Offset  int                 `binding:"omitempty,min=0" form:"offset"`
	OrderBy QueryOrder          `binding:"omitempty,oneof=created_at updated_at" form:"order_by"`
	Order   QueryOrderDirection `binding:"omitempty,oneof=asc desc" form:"order"`
}

type QueryOrder string

const (
	QueryOrderCreatedAt QueryOrder = "created_at"
	QueryOrderUpdatedAt QueryOrder = "updated_at"
)

type QueryOrderDirection string

const (
	QueryOrderDirectionAsc  QueryOrderDirection = "asc"
	QueryOrderDirectionDesc QueryOrderDirection = "desc"
)

type QueryResponse struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}
