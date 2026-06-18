package model

import "time"

type User struct {
	ID        uint      `json:"id"         gorm:"primaryKey"`
	Username  string    `json:"username"   gorm:"size:64;uniqueIndex;not null"`
	Password  string    `json:"-"          gorm:"size:256;not null"`
	Email     string    `json:"email"      gorm:"size:128;uniqueIndex;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
