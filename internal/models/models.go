package models

import "time"

type Charger struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Timezone  string    `gorm:"size:64;not null;default:UTC" json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PricingSchedule struct {
	ID            uint            `gorm:"primaryKey"`
	ChargerID     string          `gorm:"size:64;not null;index"`
	EffectiveFrom time.Time       `gorm:"type:date;not null;index"`
	EffectiveTo   *time.Time      `gorm:"type:date;index"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Periods       []PricingPeriod `gorm:"foreignKey:ScheduleID;references:ID;constraint:OnDelete:CASCADE"`
}

type PricingPeriod struct {
	ID          uint    `gorm:"primaryKey"`
	ScheduleID  uint    `gorm:"not null;index"`
	StartMinute int     `gorm:"not null"`
	EndMinute   int     `gorm:"not null"`
	PricePerKWh float64 `gorm:"type:numeric(10,4);not null"`
}
