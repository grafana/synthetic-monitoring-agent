package testserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
)

const (
	MaxBodySize       = 1024
	MaxHeaderValueLen = 256
)

var allowedHeaders = []string{
	"age",
	"allow",
	"date",
	"content-encoding",
	"content-type",
	"expires",
	"last-modified",
	"location",
}

type Settings struct {
	Scheme  string
	Method  string
	Status  int
	Headers map[string]string
	Body    []byte
}

func (s Settings) URL(addr string) string {
	scheme := s.Scheme
	if len(scheme) == 0 {
		scheme = "http"
	}

	u := url.URL{
		Scheme: scheme,
		Host:   addr,
		Path:   "/",
	}

	v := make(url.Values)

	if len(s.Method) == 0 {
		v.Set("method", http.MethodGet)
	} else {
		v.Set("method", s.Method)
	}

	v.Set("status", strconv.Itoa(s.Status))

	for name, value := range s.Headers {
		v.Add("headers", fmt.Sprintf("%s:%s", name, value))
	}

	if len(s.Body) > 0 {
		v.Add("body", base64.StdEncoding.EncodeToString(s.Body))
	}

	u.RawQuery = v.Encode()

	return u.String()
}

type Config struct {
	CertFn         string
	KeyFn          string
	AllowedDomains string
}

func New(cfg Config) *httptest.Server {
	h := httpHandler{
		allowedDomains: makeSet(strings.Split(cfg.AllowedDomains, ",")),
		allowedHeaders: makeSet(allowedHeaders),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.generator)

	return httptest.NewServer(mux)
}

type httpHandler struct {
	allowedDomains set
	allowedHeaders set
}

func (h *httpHandler) generator(w http.ResponseWriter, req *http.Request) {
	res := struct {
		StatusCode int       `json:"status"`
		Message    string    `json:"message"`
		Error      string    `json:"error,omitempty"`
		Body       io.Reader `json:"-"`
	}{
		StatusCode: http.StatusOK,
	}

	w.Header().Set("Alt-Svc", "clear")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cross-Origin-Embedder-Policy", "unsafe-none")
	w.Header().Set("Cross-Origin-Opener-Policy", "unsafe-none")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-site")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "deny")

	defer func() {
		if res.Message != "" || res.Error != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(res.StatusCode)

			enc := json.NewEncoder(w)
			err := enc.Encode(&res)
			if err != nil {
				log.Printf("E: encoding response: %s", err)
			}

			return
		}

		w.WriteHeader(res.StatusCode)

		if res.Body != nil {
			_, _ = io.Copy(w, res.Body)
		}
	}()

	if err := req.ParseForm(); err != nil {
		res.StatusCode = http.StatusBadRequest
		res.Message = "cannot parse input"
		res.Error = err.Error()

		return
	}

	if err := processMethod(req.Form["method"], req.Method); err != nil {
		res.StatusCode = http.StatusBadRequest
		res.Error = "invalid method"
		res.Message = err.Error()

		return
	}

	if err := processHeaders(req.Form["headers"], w, h.allowedDomains, h.allowedHeaders); err != nil {
		res.StatusCode = http.StatusBadRequest
		res.Message = "cannot process request"
		res.Error = err.Error()

		return
	}

	if val, found := req.Form["status"]; found {
		res.StatusCode = processStatus(val)
		if res.StatusCode/100 == 3 && h.allowedDomains.IsEmpty() {
			res.StatusCode = http.StatusBadRequest
			res.Error = "invalid status code"
			res.Message = res.Error

			return
		}
	}

	if val, found := req.Form["body"]; found {
		res.Body = io.LimitReader(base64.NewDecoder(base64.RawStdEncoding, strings.NewReader(val[0])), MaxBodySize)
	}
}

func processMethod(method []string, actual string) error {
	switch len(method) {
	case 0:
		return nil

	case 1:
		if method[0] != actual {
			return fmt.Errorf("invalid method, expected %q, actual %q", method[0], actual)
		}

	default:
		return fmt.Errorf("invalid expected method: %q", method)
	}

	return nil
}

func processHeaders(headers []string, w http.ResponseWriter, allowedDomains, allowedHeaders set) error {
	for _, h := range headers {
		key, val := splitHeader(h)
		lcKey := strings.ToLower(key)

		// Be overly strict: no CRs nor LFs allowed in header
		// values at all, not just the CRLF sequence.
		if strings.ContainsAny(val, "\r\n") {
			return fmt.Errorf("invalid header value %q", val)
		}

		if allowedHeaders.Has(lcKey) {
			return fmt.Errorf("header %q not allowed", key)
		}

		if len(val) > MaxHeaderValueLen {
			return fmt.Errorf("header %q value too long", key)
		}

		if lcKey == "location" {
			err := sanitizeLocation(val, allowedDomains)
			if err != nil {
				return fmt.Errorf("redirect to %q not allowed", val)
			}
		}

		w.Header().Set(key, val)
	}

	return nil
}

func processStatus(status []string) int {
	switch len(status) {
	case 0:
		return http.StatusBadRequest

	case 1:
		code, err := strconv.ParseInt(status[0], 10, 16)
		if err != nil {
			return http.StatusBadRequest
		}
		return int(code)
	}

	return http.StatusOK
}

func splitHeader(header string) (string, string) {
	values := strings.SplitN(header, ":", 2)
	var key, val string
	switch len(values) {
	case 2:
		val = strings.TrimSpace(values[1])
		fallthrough
	case 1:
		key = strings.TrimSpace(values[0])
	}

	return key, val
}

func sanitizeLocation(loc string, allowedDomains set) error {
	if allowedDomains.IsEmpty() {
		return errors.New("redirects not allowed")
	}

	target, err := url.Parse(loc)
	if err != nil {
		log.Printf("E: bad location %q", loc)
		return err
	}

	found := false
	for domain := range allowedDomains {
		host := strings.ToLower(target.Host)

		if !strings.HasSuffix(host, domain) {
			continue
		}

		if host == domain || strings.HasSuffix(host, "."+domain) {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("redirect to %s not allowed", target.Host)
	}

	return nil
}

type set map[string]struct{}

func makeSet(headers []string) set {
	out := make(map[string]struct{})

	for _, header := range headers {
		if len(header) > 0 {
			out[header] = struct{}{}
		}
	}

	return out
}

func (s set) IsEmpty() bool {
	return len(s) == 0
}

func (s set) Has(key string) bool {
	_, found := s[key]
	return found
}
