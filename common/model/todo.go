package model

type Todo struct {
	BaseModel
	UserID    uint   `json:"user_id"   gorm:"index;not null"`
	Title     string `json:"title"     gorm:"size:256;not null"`
	Completed bool   `json:"completed" gorm:"default:false"`
}
