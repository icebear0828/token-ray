package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var gzipWriterPool = sync.Pool{
	New: func() any {
		gw, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return gw
	},
}

// gzipMiddleware compresses compressible HTTP responses (API JSON and static SPA
// assets) when the client advertises Accept-Encoding: gzip. Binary downloads,
// bodyless status codes (204/304), HEAD requests, and already-encoded responses
// are passed through unmodified.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/download/") {
			next.ServeHTTP(w, r)
			return
		}

		addVaryAcceptEncoding(w.Header())

		if r.Method == http.MethodHead || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}

		grw := &gzipResponseWriter{ResponseWriter: w}
		defer grw.finish()
		next.ServeHTTP(grw, r)
	})
}

func addVaryAcceptEncoding(h http.Header) {
	for _, existing := range h.Values("Vary") {
		for _, token := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "Accept-Encoding") {
				return
			}
		}
	}
	h.Add("Vary", "Accept-Encoding")
}

func acceptsGzip(header string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, params, hasParams := strings.Cut(part, ";")
		name = strings.TrimSpace(name)
		if !strings.EqualFold(name, "gzip") {
			continue
		}
		if !hasParams {
			return true
		}
		q := 1.0
		for _, param := range strings.Split(params, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = parsed
				}
			}
		}
		if q > 0 {
			return true
		}
	}
	return false
}

func statusAllowsBody(code int) bool {
	if code >= 100 && code < 200 {
		return false
	}
	return code != http.StatusNoContent && code != http.StatusNotModified && code != http.StatusPartialContent
}

type gzipResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
	decided     bool
	compress    bool
	gw          *gzip.Writer
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.statusCode = code
	g.wroteHeader = true
	if !statusAllowsBody(code) || g.Header().Get("Content-Encoding") != "" {
		g.decided = true
		g.compress = false
		g.ResponseWriter.WriteHeader(code)
	}
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !g.decided {
		g.decided = true
		if g.statusCode == 0 {
			g.statusCode = http.StatusOK
		}
		if statusAllowsBody(g.statusCode) && g.Header().Get("Content-Encoding") == "" {
			g.compress = true
			g.Header().Del("Content-Length")
			g.Header().Set("Content-Encoding", "gzip")
			g.ResponseWriter.WriteHeader(g.statusCode)
			gw := gzipWriterPool.Get().(*gzip.Writer)
			gw.Reset(g.ResponseWriter)
			g.gw = gw
		} else {
			g.compress = false
			g.ResponseWriter.WriteHeader(g.statusCode)
		}
	}
	if g.compress {
		return g.gw.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipResponseWriter) Flush() {
	if !g.decided {
		g.decided = true
		if g.statusCode == 0 {
			g.statusCode = http.StatusOK
		}
		g.compress = false
		g.ResponseWriter.WriteHeader(g.statusCode)
	}
	if g.compress && g.gw != nil {
		_ = g.gw.Flush()
	}
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

func (g *gzipResponseWriter) finish() {
	if g.gw != nil {
		_ = g.gw.Close()
		g.gw.Reset(io.Discard)
		gzipWriterPool.Put(g.gw)
		g.gw = nil
		return
	}
	if !g.decided && g.wroteHeader {
		g.decided = true
		g.ResponseWriter.WriteHeader(g.statusCode)
	}
}
