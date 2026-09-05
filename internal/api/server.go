package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/senton89/mexc-ticker/internal/model"
)

const maxRange = 7 * 24 * time.Hour

// CandleReader — то, что API нужно от хранилища. Интерфейс объявлен здесь,
// у потребителя: api не зависит от pgx и тестируется с фейком.
type CandleReader interface {
	QueryCandles(ctx context.Context, symbol string, from, to time.Time) ([]model.Candle, error)
	Ping(ctx context.Context) error
}

// Server — HTTP-обвязка над хранилищем свечей.
type Server struct {
	store CandleReader
	log   *slog.Logger
}

// New создаёт сервер.
func New(store CandleReader, log *slog.Logger) *Server {
	return &Server{store: store, log: log}
}

// Handler собирает маршруты и оборачивает их логирующим middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /candles", s.handleCandles)
	return logging(s.log, mux)
}

// candleDTO — форма свечи в JSON-ответе; отделена от model, чтобы менять API независимо.
type candleDTO struct {
	Symbol string    `json:"symbol"`
	Start  time.Time `json:"start"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCandles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	symbol := strings.ToUpper(strings.TrimSpace(q.Get("symbol")))
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}

	now := time.Now().UTC()
	to, err := parseTime(q.Get("to"), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad to: RFC3339 expected")
		return
	}
	from, err := parseTime(q.Get("from"), to.Add(-time.Hour))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad from: RFC3339 expected")
		return
	}
	if !from.Before(to) {
		writeError(w, http.StatusBadRequest, "from must be before to")
		return
	}
	if to.Sub(from) > maxRange {
		writeError(w, http.StatusBadRequest, "range exceeds 7 days")
		return
	}

	candles, err := s.store.QueryCandles(r.Context(), symbol, from, to)
	if err != nil {
		s.log.Error("query candles", "symbol", symbol, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]candleDTO, 0, len(candles))
	for _, c := range candles {
		out = append(out, candleDTO{
			Symbol: c.Symbol, Start: c.Start,
			Open: c.Open, High: c.High, Low: c.Low, Close: c.Close,
			Volume: c.Volume,
		})
	}
	writeJSON(w, http.StatusOK, out)

}

// parseTime разбирает RFC3339; пустая строка → значение по умолчанию.
func parseTime(s string, def time.Time) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	return time.Parse(time.RFC3339, s)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // ошибка записи клиенту — уже нечего делать
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
