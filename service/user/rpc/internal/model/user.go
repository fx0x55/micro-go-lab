package model

import "gorm.io/gorm"

// User 是 user-rpc 用户域的私有数据模型。仅 user-rpc 直接操作此结构；
// 其他服务只能通过 gRPC 契约（pb）感知用户字段，不感知存储形态。
type User struct {
	gorm.Model
	Username string `json:"username" gorm:"size:64;uniqueIndex;not null"`
	Password string `json:"-"        gorm:"size:256;not null"`
	Email    string `json:"email"    gorm:"size:128;uniqueIndex;not null"`
}
