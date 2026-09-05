package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/senton89/mexc-ticker/internal/model"
)

// rawTrade — сделка в формате ответа MEXC GET /api/v3/trades. Только нужные поля.
type rawTrade struct {
	Price     string `json:"price"`
	Qty       string `json:"qty"`
	Time      int64  `json:"time"`
	TradeType string `json:"tradeType"`
}

// Client опрашивает REST MEXC по списку символов и отдаёт сделки в канал.
type Client struct {
	http     *http.Client
	baseURL  string
	symbols  []string
	interval time.Duration
	limit    int
	log      *slog.Logger
}

// New создаёт клиент. baseURL без завершающего слэша, например https://api.mexc.com.
func New(baseURL string, symbols []string, interval time.Duration, log *slog.Logger) *Client {
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		baseURL:  baseURL,
		symbols:  symbols,
		interval: interval,
		limit:    500,
		log:      log,
	}
}

// Run запускает по горутине на символ и блокируется до отмены ctx.
// Run — единственный писатель в out и закрывает его при выходе.
// Возвращает ctx.Err() после завершения всех горутин.
func (c *Client) Run(ctx context.Context, out chan<- model.Trade) error {
	defer close(out)

	var wg sync.WaitGroup
	for _, sym := range c.symbols {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.pollLoop(ctx, sym, out)
		}()
	}
	wg.Wait()
	return ctx.Err()
}

// pollLoop — цикл одного символа: тикер → poll → дедуп → out.
func (c *Client) pollLoop(ctx context.Context, symbol string, out chan<- model.Trade) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	var lastTime int64
	seen := make(map[string]struct{})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			raws, err := c.poll(ctx, symbol)
			if err != nil {
				if ctx.Err() != nil {
					return // отмена контекста — штатная остановка, не ошибка опроса
				}
				c.log.Warn("poll failed", "symbol", symbol, "err", err)
				continue
			}
			slices.Reverse(raws) // MEXC отдаёт от новых к старым; нам нужен хронологический порядок

			for _, r := range raws {
				fp := fingerprint(r)
				switch {
				case r.Time < lastTime:
					continue
				case r.Time == lastTime:
					if _, ok := seen[fp]; ok {
						continue
					}
					seen[fp] = struct{}{}
				default: // r.Time > lastTime
					lastTime = r.Time
					clear(seen)
					seen[fp] = struct{}{}
				}

				t, err := toTrade(symbol, r)
				if err != nil {
					c.log.Warn("bad trade", "symbol", symbol, "err", err)
					continue
				}

				select {
				case out <- t:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// poll делает один HTTP-запрос и возвращает сырые сделки.
func (c *Client) poll(ctx context.Context, symbol string) ([]rawTrade, error) {
	url := fmt.Sprintf("%s/api/v3/trades?symbol=%s&limit=%d", c.baseURL, symbol, c.limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("poll %s: build request: %w", symbol, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll %s: %w", symbol, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poll %s: Status code %d", symbol, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("poll %s: read body: %w", symbol, err)
	}

	var raws []rawTrade
	err = json.Unmarshal(body, &raws)
	if err != nil {
		return nil, fmt.Errorf("poll %s: decode: %w", symbol, err)
	}

	return raws, nil
}

// toTrade конвертирует сырую сделку во внутренний тип.
func toTrade(symbol string, r rawTrade) (model.Trade, error) {
	price, err := strconv.ParseFloat(r.Price, 64)
	if err != nil {
		return model.Trade{}, fmt.Errorf("parse price %q: %w", r.Price, err)
	}
	qty, err := strconv.ParseFloat(r.Qty, 64)
	if err != nil {
		return model.Trade{}, fmt.Errorf("parse qty %q: %w", r.Qty, err)
	}
	return model.Trade{
		Symbol: symbol,
		Price:  price,
		Qty:    qty,
		Time:   time.UnixMilli(r.Time),
	}, nil
}

// fingerprint — отпечаток сделки для дедупа в пределах одной миллисекунды.
func fingerprint(r rawTrade) string {
	return r.Price + "|" + r.Qty + "|" + r.TradeType
}
