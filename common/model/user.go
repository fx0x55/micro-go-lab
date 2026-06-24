package model

type User struct {
	BaseModel
	Username string `json:"username" gorm:"size:64;uniqueIndex;not null"`
	Password string `json:"-"        gorm:"size:256;not null"`
	Email    string `json:"email"    gorm:"size:128;uniqueIndex;not null"`
}
