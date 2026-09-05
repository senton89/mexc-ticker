package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusWriter запоминает код ответа: стандартный ResponseWriter его не отдаёт.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logging пишет одну строку slog на запрос: метод, путь, статус, длительность.
func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "dur", time.Since(start))
	})
}
