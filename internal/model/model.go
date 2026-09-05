package model

import "time"

// Trade — одна сделка с биржи, уже приведённая к внутренним типам.
type Trade struct {
	Symbol string
	Price  float64
	Qty    float64
	Time   time.Time
}

type Candle struct {
	Symbol string
	Start  time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Trades int
}
