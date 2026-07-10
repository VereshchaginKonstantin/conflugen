package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// TestClientFollowsEdgeGatewayRedirect воспроизводит поведение nginx-based edge gateway
// перед Confluence, который на первом запросе ставит cookie через 307 + Set-Cookie и
// редиректит на тот же URL с маркером __rr=1. Клиент обязан хранить cookie между
// запросами, иначе будет петля до 10 редиректов.
func TestClientFollowsEdgeGatewayRedirect(t *testing.T) {
	t.Parallel()

	const gatewayCookie = "__Secure-Gateway"
	var requestCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		if _, err := r.Cookie(gatewayCookie); err != nil {
			http.SetCookie(w, &http.Cookie{
				Name:     gatewayCookie,
				Value:    "test-edge-token",
				Path:     "/",
				HttpOnly: true,
			})
			u := *r.URL
			q := u.Query()
			q.Set("__rr", "1")
			u.RawQuery = q.Encode()
			w.Header().Set("Location", u.String())
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1","title":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := goconfluence.NewAPI(srv.URL+"/rest/api", "", "test-token")
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	c.Client.Jar = jar

	if _, err := c.GetContentByID("1", goconfluence.ContentQuery{}); err != nil {
		t.Fatalf("GetContentByID: %v", err)
	}

	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("ожидалось 2 запроса (307 + 200), получено %d", got)
	}
}

// TestClientWithoutCookieJarLoops показывает, что без CookieJar тот же gateway
// заставляет клиента упереться в лимит редиректов — это и есть оригинальный баг.
func TestClientWithoutCookieJarLoops(t *testing.T) {
	t.Parallel()

	const gatewayCookie = "__Secure-Gateway"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(gatewayCookie); err != nil {
			http.SetCookie(w, &http.Cookie{
				Name: gatewayCookie, Value: "x", Path: "/", HttpOnly: true,
			})
			u := *r.URL
			q := u.Query()
			q.Set("__rr", "1")
			u.RawQuery = q.Encode()
			w.Header().Set("Location", u.String())
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c, err := goconfluence.NewAPI(srv.URL+"/rest/api", "", "test-token")
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}

	_, err = c.GetContentByID("1", goconfluence.ContentQuery{})
	if err == nil {
		t.Fatalf("ожидалась ошибка stopped after 10 redirects, ошибки нет")
	}
	var urlErr *url.Error
	if !asURLErr(err, &urlErr) || !strings.Contains(urlErr.Err.Error(), "stopped after") {
		t.Fatalf("ожидалась ошибка вида 'stopped after N redirects', получено: %v", err)
	}
}

// TestBasicAuthWhenUsernameProvided проверяет, что при непустом username
// библиотека goconfluence отправляет Basic auth вместо Bearer.
func TestBasicAuthWhenUsernameProvided(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","title":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := goconfluence.NewAPI(srv.URL+"/rest/api", "alice", "s3cret")
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}
	if _, err := c.GetContentByID("1", goconfluence.ContentQuery{}); err != nil {
		t.Fatalf("GetContentByID: %v", err)
	}

	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("ожидался Basic auth, получено: %q", gotAuth)
	}
}

// TestBearerAuthWhenUsernameEmpty проверяет, что при пустом username библиотека
// шлёт Bearer — это поведение по умолчанию для conflugen.
func TestBearerAuthWhenUsernameEmpty(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","title":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := goconfluence.NewAPI(srv.URL+"/rest/api", "", "pat-token")
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}
	if _, err := c.GetContentByID("1", goconfluence.ContentQuery{}); err != nil {
		t.Fatalf("GetContentByID: %v", err)
	}

	if gotAuth != "Bearer pat-token" {
		t.Fatalf("ожидался Bearer pat-token, получено: %q", gotAuth)
	}
}

// TestXSRFBypassTransportSetsHeader проверяет, что xsrfBypassTransport
// навешивает X-Atlassian-Token: no-check на каждый исходящий запрос.
// Без этого Confluence Server отвечает 403 на любой state-changing вызов.
func TestXSRFBypassTransportSetsHeader(t *testing.T) {
	t.Parallel()

	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Atlassian-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","title":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := goconfluence.NewAPI(srv.URL+"/rest/api", "", "test-token")
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}
	base := c.Client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.Client.Transport = &xsrfBypassTransport{base: base}

	if _, err := c.GetContentByID("1", goconfluence.ContentQuery{}); err != nil {
		t.Fatalf("GetContentByID: %v", err)
	}
	if gotHeader != "no-check" {
		t.Fatalf("ожидался X-Atlassian-Token=no-check, получено %q", gotHeader)
	}
}

// TestXSRFBypassTransportDoesNotMutateOriginal страхует контракт http.RoundTripper:
// RoundTrip не должен изменять переданный *http.Request, должен работать с копией.
func TestXSRFBypassTransportDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	tr := &xsrfBypassTransport{base: http.DefaultTransport}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_ = resp.Body.Close()

	if got := req.Header.Get("X-Atlassian-Token"); got != "" {
		t.Fatalf("оригинальный req не должен меняться, получено X-Atlassian-Token=%q", got)
	}
}

// TestUserAgentTransportSetsHeader проверяет, что userAgentTransport ставит
// заданный User-Agent (вместо дефолтного Go-http-client/1.1, который для
// корпоративных antibot/WAF — известная сигнатура бота).
func TestUserAgentTransportSetsHeader(t *testing.T) {
	t.Parallel()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	const want = "conflugen-test/0.0.1"
	tr := &userAgentTransport{userAgent: want, base: http.DefaultTransport}
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_ = resp.Body.Close()

	if gotUA != want {
		t.Fatalf("ожидался User-Agent=%q, получено %q", want, gotUA)
	}
}

// TestUserAgentTransportRespectsExisting проверяет, что transport не
// перетирает уже заданный User-Agent — это даёт возможность вызывающему коду
// (или внешнему конфигу) полностью контролировать значение.
func TestUserAgentTransportRespectsExisting(t *testing.T) {
	t.Parallel()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tr := &userAgentTransport{userAgent: "should-not-be-used/0.0", base: http.DefaultTransport}
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	const custom = "my-custom-agent/1.2.3"
	req.Header.Set("User-Agent", custom)

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_ = resp.Body.Close()

	if gotUA != custom {
		t.Fatalf("заранее заданный User-Agent должен остаться %q, получено %q", custom, gotUA)
	}
}

// TestUserAgentTransportDoesNotMutateOriginal — контракт RoundTripper:
// не мутируем исходный *http.Request.
func TestUserAgentTransportDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	tr := &userAgentTransport{userAgent: "conflugen-test/0.0.1", base: http.DefaultTransport}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_ = resp.Body.Close()

	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("оригинальный req не должен меняться, получено User-Agent=%q", got)
	}
}

// TestErrorBodyLoggingTransportLogsBodyOn4xx проверяет, что при 4xx ответе
// transport печатает в log тело ответа (поле message от Confluence).
func TestErrorBodyLoggingTransportLogsBodyOn4xx(t *testing.T) {
	const respBody = `{"statusCode":403,"message":"User not permitted to create"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)

	var logBuf bytes.Buffer
	origOutput := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOutput)
		log.SetFlags(origFlags)
	})

	client := &http.Client{Transport: &errorBodyLoggingTransport{base: http.DefaultTransport}}
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/rest/api/content/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	logged := logBuf.String()
	if !strings.Contains(logged, "403") {
		t.Fatalf("в log нет статуса 403: %q", logged)
	}
	if !strings.Contains(logged, "User not permitted to create") {
		t.Fatalf("в log нет тела ответа: %q", logged)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll resp.Body: %v", err)
	}
	if string(got) != respBody {
		t.Fatalf("тело должно быть доступно для повторного чтения, ожидалось %q, получено %q", respBody, string(got))
	}
}

// TestErrorBodyLoggingTransportSilentOn2xx проверяет, что при 2xx ответе
// transport ничего в log не пишет.
func TestErrorBodyLoggingTransportSilentOn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	var logBuf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(origOutput) })

	client := &http.Client{Transport: &errorBodyLoggingTransport{base: http.DefaultTransport}}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/ok", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if logBuf.Len() != 0 {
		t.Fatalf("на 2xx не должно быть лога, получено: %q", logBuf.String())
	}
}

func asURLErr(err error, target **url.Error) bool {
	for err != nil {
		if e, ok := err.(*url.Error); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
