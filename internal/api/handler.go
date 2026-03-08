package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"presto/internal/models"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
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

type bulkUpdatePricingRequest struct {
	ChargerIDs    []string        `json:"charger_ids"`
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

func (h *Handler) HandleCreateCharger(w http.ResponseWriter, r *http.Request) {
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

	charger := models.Charger{
		ID:       strings.TrimSpace(req.ID),
		Name:     strings.TrimSpace(req.Name),
		Timezone: tz,
	}

	if err := h.db.WithContext(r.Context()).Create(&charger).Error; err != nil {
		writeError(w, http.StatusConflict, "charger already exists")
		return
	}

	writeJSON(w, http.StatusCreated, charger)
}

func (h *Handler) HandleUpdatePricing(w http.ResponseWriter, r *http.Request) {
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
	var charger models.Charger
	if err := h.db.WithContext(ctx).First(&charger, "id = ?", chargerID).Error; err != nil {
		writeError(w, http.StatusNotFound, "charger not found")
		return
	}

	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		s := models.PricingSchedule{
			ChargerID:     charger.ID,
			EffectiveFrom: effectiveFrom,
			EffectiveTo:   effectiveTo,
		}
		if err := tx.Create(&s).Error; err != nil {
			return err
		}

		periodRows := make([]models.PricingPeriod, 0, len(periods))
		for _, p := range periods {
			periodRows = append(periodRows, models.PricingPeriod{
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

func (h *Handler) HandleGetPricing(w http.ResponseWriter, r *http.Request) {
	chargerID := strings.TrimSpace(chi.URLParam(r, "chargerID"))
	if chargerID == "" {
		writeError(w, http.StatusBadRequest, "chargerID is required")
		return
	}

	var charger models.Charger
	if err := h.db.WithContext(r.Context()).First(&charger, "id = ?", chargerID).Error; err != nil {
		writeError(w, http.StatusNotFound, "charger not found")
		return
	}

	dateVal, minuteOfDay, err := resolveLocalDateAndMinute(r, charger.Timezone)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	schedule, err := findScheduleForDate(r.Context(), h.db, chargerID, dateVal)
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

func (h *Handler) HandleBulkUpdatePricing(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[bulkUpdatePricingRequest](w, r)
	if !ok {
		return
	}
	if len(req.ChargerIDs) == 0 {
		writeError(w, http.StatusBadRequest, "charger_ids are required")
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

	cleanIDs := make([]string, 0, len(req.ChargerIDs))
	seen := make(map[string]struct{}, len(req.ChargerIDs))
	for _, id := range req.ChargerIDs {
		v := strings.TrimSpace(id)
		if v == "" {
			writeError(w, http.StatusBadRequest, "charger_ids cannot contain empty values")
			return
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		cleanIDs = append(cleanIDs, v)
	}

	var chargers []models.Charger
	if err := h.db.WithContext(r.Context()).Where("id IN ?", cleanIDs).Find(&chargers).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chargers")
		return
	}
	if len(chargers) != len(cleanIDs) {
		writeError(w, http.StatusNotFound, "one or more chargers not found")
		return
	}

	if err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		for _, charger := range chargers {
			s := models.PricingSchedule{
				ChargerID:     charger.ID,
				EffectiveFrom: effectiveFrom,
				EffectiveTo:   effectiveTo,
			}
			if err := tx.Create(&s).Error; err != nil {
				return err
			}

			periodRows := make([]models.PricingPeriod, 0, len(periods))
			for _, p := range periods {
				periodRows = append(periodRows, models.PricingPeriod{
					ScheduleID:  s.ID,
					StartMinute: p.StartMinute,
					EndMinute:   p.EndMinute,
					PricePerKWh: p.PricePerKWh,
				})
			}
			if err := tx.Create(&periodRows).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to bulk update pricing")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":          "bulk pricing schedules updated",
		"updated_chargers": len(cleanIDs),
	})
}

func findScheduleForDate(ctx context.Context, db *gorm.DB, chargerID string, dateVal time.Time) (models.PricingSchedule, error) {
	var schedule models.PricingSchedule
	err := db.WithContext(ctx).
		Preload("Periods", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("start_minute ASC")
		}).
		Where("charger_id = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)", chargerID, dateVal, dateVal).
		Order("effective_from DESC").
		First(&schedule).Error
	return schedule, err
}

func validateAndMapPeriods(in []periodRequest) ([]models.PricingPeriod, error) {
	out := make([]models.PricingPeriod, 0, len(in))
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
		out = append(out, models.PricingPeriod{
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

func resolveLocalDateAndMinute(r *http.Request, timezone string) (time.Time, int, error) {
	queryDate := strings.TrimSpace(r.URL.Query().Get("date"))
	queryTime := strings.TrimSpace(r.URL.Query().Get("time"))
	queryTimestamp := strings.TrimSpace(r.URL.Query().Get("timestamp"))

	if queryTimestamp != "" {
		if queryDate != "" || queryTime != "" {
			return time.Time{}, 0, errors.New("use either timestamp or date+time, not both")
		}
		t, err := time.Parse(time.RFC3339, queryTimestamp)
		if err != nil {
			return time.Time{}, 0, errors.New("timestamp must be RFC3339, e.g. 2026-03-08T14:30:00Z")
		}
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, 0, errors.New("charger timezone cannot be loaded")
		}
		local := t.In(loc)
		dateOnly, err := parseDate(local.Format("2006-01-02"))
		if err != nil {
			return time.Time{}, 0, errors.New("failed to derive local date from timestamp")
		}
		return dateOnly, local.Hour()*60 + local.Minute(), nil
	}

	if queryDate == "" || queryTime == "" {
		return time.Time{}, 0, errors.New("provide either timestamp or both date and time query params")
	}
	dateVal, err := parseDate(queryDate)
	if err != nil {
		return time.Time{}, 0, errors.New("date must be YYYY-MM-DD")
	}
	minuteOfDay, err := parseHHMM(queryTime)
	if err != nil {
		return time.Time{}, 0, errors.New("time must be HH:MM in 24-hour format")
	}
	return dateVal, minuteOfDay, nil
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
