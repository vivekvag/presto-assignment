package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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
	Periods       []PricingPeriod `gorm:"constraint:OnDelete:CASCADE"`
}

type PricingPeriod struct {
	ID          uint    `gorm:"primaryKey"`
	ScheduleID  uint    `gorm:"not null;index"`
	StartMinute int     `gorm:"not null"`
	EndMinute   int     `gorm:"not null"`
	PricePerKWh float64 `gorm:"type:numeric(10,4);not null"`
}

type createChargerRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

type updatePricingRequest struct {
	EffectiveFrom string          `json:"effective_from"`
	EffectiveTo   *string         `json:"effective_to"`
	Periods       []periodRequest `json:"periods"`
}

type periodRequest struct {
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	PricePerKWh float64 `json:"price_per_kwh"`
}

type pricingResponse struct {
	ChargerID     string           `json:"charger_id"`
	EffectiveFrom string           `json:"effective_from"`
	EffectiveTo   *string          `json:"effective_to,omitempty"`
	Periods       []periodResponse `json:"periods"`
	CurrentPeriod *periodResponse  `json:"current_period,omitempty"`
}

type periodResponse struct {
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	PricePerKWh float64 `json:"price_per_kwh"`
}

type app struct {
	db *gorm.DB
}

func main() {
	dsn := getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/presto?sslmode=disable")
	port := getenv("PORT", "8080")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	if err := db.AutoMigrate(&Charger{}, &PricingSchedule{}, &PricingPeriod{}); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}

	a := &app{db: db}

	r := chi.NewRouter()
	r.Post("/api/v1/chargers", a.handleCreateCharger)
	r.Put("/api/v1/chargers/{chargerID}/pricing", a.handleUpdatePricing)
	r.Get("/api/v1/chargers/{chargerID}/pricing", a.handleGetPricing)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("server listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func (a *app) handleCreateCharger(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[createChargerRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "id and name are required")
		return
	}

	tz := strings.TrimSpace(req.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		writeError(w, http.StatusBadRequest, "invalid timezone")
		return
	}

	charger := Charger{
		ID:       strings.TrimSpace(req.ID),
		Name:     strings.TrimSpace(req.Name),
		Timezone: tz,
	}

	if err := a.db.WithContext(r.Context()).Create(&charger).Error; err != nil {
		writeError(w, http.StatusConflict, "charger already exists")
		return
	}

	writeJSON(w, http.StatusCreated, charger)
}

func (a *app) handleUpdatePricing(w http.ResponseWriter, r *http.Request) {
	chargerID := strings.TrimSpace(chi.URLParam(r, "chargerID"))
	if chargerID == "" {
		writeError(w, http.StatusBadRequest, "chargerID is required")
		return
	}

	req, ok := decodeJSON[updatePricingRequest](w, r)
	if !ok {
		return
	}

	if len(req.Periods) == 0 {
		writeError(w, http.StatusBadRequest, "periods are required")
		return
	}

	effectiveFrom, err := parseDate(req.EffectiveFrom)
	if err != nil {
		writeError(w, http.StatusBadRequest, "effective_from must be YYYY-MM-DD")
		return
	}

	var effectiveTo *time.Time
	if req.EffectiveTo != nil && strings.TrimSpace(*req.EffectiveTo) != "" {
		et, err := parseDate(*req.EffectiveTo)
		if err != nil {
			writeError(w, http.StatusBadRequest, "effective_to must be YYYY-MM-DD")
			return
		}
		if et.Before(effectiveFrom) {
			writeError(w, http.StatusBadRequest, "effective_to cannot be before effective_from")
			return
		}
		effectiveTo = &et
	}

	periods, err := validateAndMapPeriods(req.Periods)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	var charger Charger
	if err := a.db.WithContext(ctx).First(&charger, "id = ?", chargerID).Error; err != nil {
		writeError(w, http.StatusNotFound, "charger not found")
		return
	}

	if err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		s := PricingSchedule{
			ChargerID:     charger.ID,
			EffectiveFrom: effectiveFrom,
			EffectiveTo:   effectiveTo,
		}
		if err := tx.Create(&s).Error; err != nil {
			return err
		}

		periodRows := make([]PricingPeriod, 0, len(periods))
		for _, p := range periods {
			periodRows = append(periodRows, PricingPeriod{
				ScheduleID:  s.ID,
				StartMinute: p.StartMinute,
				EndMinute:   p.EndMinute,
				PricePerKWh: p.PricePerKWh,
			})
		}
		return tx.Create(&periodRows).Error
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update pricing")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "pricing schedule updated",
	})
}

func (a *app) handleGetPricing(w http.ResponseWriter, r *http.Request) {
	chargerID := strings.TrimSpace(chi.URLParam(r, "chargerID"))
	if chargerID == "" {
		writeError(w, http.StatusBadRequest, "chargerID is required")
		return
	}

	queryDate := strings.TrimSpace(r.URL.Query().Get("date"))
	queryTime := strings.TrimSpace(r.URL.Query().Get("time"))
	if queryDate == "" || queryTime == "" {
		writeError(w, http.StatusBadRequest, "date and time query params are required")
		return
	}

	dateVal, err := parseDate(queryDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	minuteOfDay, err := parseHHMM(queryTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "time must be HH:MM in 24-hour format")
		return
	}

	var charger Charger
	if err := a.db.WithContext(r.Context()).First(&charger, "id = ?", chargerID).Error; err != nil {
		writeError(w, http.StatusNotFound, "charger not found")
		return
	}

	schedule, err := findScheduleForDate(r.Context(), a.db, chargerID, dateVal)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusNotFound, "no pricing schedule found for date")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch schedule")
		return
	}

	resp := pricingResponse{
		ChargerID:     charger.ID,
		EffectiveFrom: schedule.EffectiveFrom.Format("2006-01-02"),
		Periods:       make([]periodResponse, 0, len(schedule.Periods)),
	}
	if schedule.EffectiveTo != nil {
		v := schedule.EffectiveTo.Format("2006-01-02")
		resp.EffectiveTo = &v
	}

	for _, p := range schedule.Periods {
		pr := periodResponse{
			StartTime:   minuteToHHMM(p.StartMinute),
			EndTime:     minuteToHHMM(p.EndMinute),
			PricePerKWh: p.PricePerKWh,
		}
		resp.Periods = append(resp.Periods, pr)
		if minuteOfDay >= p.StartMinute && minuteOfDay < p.EndMinute {
			cp := pr
			resp.CurrentPeriod = &cp
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func findScheduleForDate(ctx context.Context, db *gorm.DB, chargerID string, dateVal time.Time) (PricingSchedule, error) {
	var schedule PricingSchedule
	err := db.WithContext(ctx).
		Preload("Periods", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("start_minute ASC")
		}).
		Where("charger_id = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)", chargerID, dateVal, dateVal).
		Order("effective_from DESC").
		First(&schedule).Error
	return schedule, err
}

func validateAndMapPeriods(in []periodRequest) ([]PricingPeriod, error) {
	out := make([]PricingPeriod, 0, len(in))
	for _, p := range in {
		start, err := parseHHMM(p.StartTime)
		if err != nil {
			return nil, fmt.Errorf("invalid start_time: %s", p.StartTime)
		}
		end, err := parseHHMM(p.EndTime)
		if err != nil {
			return nil, fmt.Errorf("invalid end_time: %s", p.EndTime)
		}
		if end <= start {
			return nil, errors.New("each period must have end_time greater than start_time")
		}
		if p.PricePerKWh < 0 {
			return nil, errors.New("price_per_kwh must be >= 0")
		}
		out = append(out, PricingPeriod{
			StartMinute: start,
			EndMinute:   end,
			PricePerKWh: p.PricePerKWh,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartMinute < out[j].StartMinute
	})

	for i := 1; i < len(out); i++ {
		if out[i].StartMinute < out[i-1].EndMinute {
			return nil, errors.New("periods cannot overlap")
		}
	}

	return out, nil
}

func parseDate(v string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(v))
}

func parseHHMM(v string) (int, error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, errors.New("invalid format")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, errors.New("out of range")
	}
	return hour*60 + minute, nil
}

func minuteToHHMM(total int) string {
	h := total / 60
	m := total % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return v, false
	}
	return v, true
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}
