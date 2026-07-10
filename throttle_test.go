package main

import (
	"net/http"
	"testing"
	"time"
)

// fakeRoundTripper — минимальный base-транспорт: считает вызовы, мгновенно
// возвращает 200, в сеть не ходит.
type fakeRoundTripper struct{ calls int }

func (f *fakeRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

func TestThrottleTransport_SpacesConsecutiveRequests(t *testing.T) {
	const interval = 40 * time.Millisecond
	base := &fakeRoundTripper{}
	tr := &throttleTransport{base: base, interval: interval}
	req, _ := http.NewRequest(http.MethodGet, "http://example/x", nil)

	// Первый запрос идёт без задержки (last ещё нулевой).
	start := time.Now()
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("первый запрос: неожиданная ошибка: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= interval {
		t.Fatalf("первый запрос не должен ждать, ждал %v (>= %v)", elapsed, interval)
	}

	// Второй запрос подряд должен выждать минимум interval.
	start = time.Now()
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("второй запрос: неожиданная ошибка: %v", err)
	}
	if elapsed := time.Since(start); elapsed < interval {
		t.Fatalf("второй запрос должен выждать >= %v, ждал %v", interval, elapsed)
	}

	if base.calls != 2 {
		t.Fatalf("ожидалось 2 вызова base-транспорта, получено %d", base.calls)
	}
}

// При паузе >= interval между вызовами троттлинг не добавляет задержку.
func TestThrottleTransport_NoWaitAfterIdle(t *testing.T) {
	const interval = 20 * time.Millisecond
	tr := &throttleTransport{base: &fakeRoundTripper{}, interval: interval}
	req, _ := http.NewRequest(http.MethodGet, "http://example/x", nil)

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	time.Sleep(interval + 10*time.Millisecond)

	start := time.Now()
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= interval {
		t.Fatalf("после простоя запрос не должен ждать, ждал %v (>= %v)", elapsed, interval)
	}
}
