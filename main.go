package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	targetHost   = flag.String("target", "fingerprints.bablosoft.com", "origin host to proxy to")
	listenAddr   = flag.String("listen", ":9999", "address to listen on")
	timeout      = flag.Duration("timeout", 30*time.Second, "timeout for origin requests")
	maxRedirects = flag.Int("maxredir", 10, "maximum redirects to follow manually")
	verbose      = flag.Bool("v", true, "verbose logging")
)

func main() {
	flag.Parse()

	logger := log.Default()
	if !*verbose {
		logger.SetFlags(0)
	}

	handler := newProxyHandler(*targetHost, *timeout, *maxRedirects, logger)

	server := &http.Server{
		Addr:         *listenAddr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	logger.Printf("Proxy started on %s -> %s", *listenAddr, *targetHost)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server failed: %v", err)
	}
}

type proxy struct {
	target     string
	httpClient *http.Client
	logger     *log.Logger
	maxRedir   int
}

func newProxyHandler(target string, timeout time.Duration, maxRedir int, logger *log.Logger) http.Handler {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// запрещаем авто-редиректы — делаем сами
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 64,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	return &proxy{
		target:     target,
		httpClient: client,
		logger:     logger,
		maxRedir:   maxRedir,
	}
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.logger != nil {
		p.logger.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.RequestURI())
	}

	// читаем тело, чтобы можно было переиспользовать при редиректах
	var bodyBuf []byte
	if r.Body != nil {
		bodyBuf, _ = io.ReadAll(r.Body)
	}

	resp, err := p.followRedirects(r, bodyBuf, p.maxRedir)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		if p.logger != nil {
			p.logger.Printf("proxy error: %v", err)
		}
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	removeHopByHop(w.Header())
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// followRedirects — вручную обходит 3xx до maxRedirects раз
func (p *proxy) followRedirects(r *http.Request, body []byte, max int) (*http.Response, error) {
	currentURL := &url.URL{
		Scheme:   "https",
		Host:     p.target,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	method := r.Method

	for i := 0; i < max; i++ {
		req, err := http.NewRequestWithContext(context.Background(), method, currentURL.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		copyHeader(req.Header, r.Header)
		req.Host = p.target
		removeHopByHop(req.Header)

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		// если не 3xx — возвращаем
		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			return resp, nil
		}

		loc := resp.Header.Get("Location")
		if loc == "" {
			return resp, nil
		}

		_ = resp.Body.Close()

		newURL, err := currentURL.Parse(loc)
		if err != nil {
			if p.logger != nil {
				p.logger.Printf("invalid redirect URL: %s", loc)
			}
			return resp, nil
		}

		if p.logger != nil {
			p.logger.Printf("redirect %d -> %s", i+1, newURL.String())
		}

		currentURL = newURL
		// HTTP 303/302 → смена метода на GET
		if resp.StatusCode == 303 || resp.StatusCode == 302 {
			method = http.MethodGet
			body = nil
		}
	}

	return nil, &url.Error{Op: "redirect", URL: currentURL.String(), Err: context.DeadlineExceeded}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

var hopByHop = []string{
	"Connection", "Proxy-Connection", "Keep-Alive",
	"Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailers", "Transfer-Encoding", "Upgrade",
}

func removeHopByHop(h http.Header) {
	for _, hname := range hopByHop {
		h.Del(hname)
	}
	if conns, ok := h["Connection"]; ok {
		for _, c := range conns {
			for _, p := range strings.Split(c, ",") {
				h.Del(strings.TrimSpace(p))
			}
		}
		h.Del("Connection")
	}
}
