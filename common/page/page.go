package page

import "math"

// Request 是通用的分页请求参数。
type Request struct {
	Page     int `form:"page,default=1" validate:"min=1"`
	PageSize int `form:"page_size,default=20" validate:"min=1,max=100"`
}

// Offset 计算数据库查询的偏移量。
func (r *Request) Offset() int {
	return (r.Page - 1) * r.PageSize
}

// Result 是通用的分页响应结构。
type Result struct {
	Items    interface{} `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Pages    int         `json:"pages"`
}

// NewResult 创建分页响应，自动计算总页数。
func NewResult(items interface{}, total int64, pageNum, pageSize int) *Result {
	pages := 0
	if pageSize > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}
	return &Result{
		Items:    items,
		Total:    total,
		Page:     pageNum,
		PageSize: pageSize,
		Pages:    pages,
	}
}
