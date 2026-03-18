package api

import (
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// spaHandler serves the embedded SPA assets. For any path that doesn't match
// a real file, it falls back to index.html so client-side routing works.
func (s *Server) spaHandler(c *gin.Context) {
	if s.webFS == nil {
		c.String(http.StatusNotFound, "frontend not built; run: cd web && npm run build")
		return
	}

	// Strip the /ui prefix; Gin's *filepath param includes the leading slash.
	reqPath := c.Param("filepath")
	if reqPath == "" || reqPath == "/" {
		reqPath = "/index.html"
	}
	reqPath = strings.TrimPrefix(reqPath, "/")

	// Try to open the requested file.
	f, err := s.webFS.Open(reqPath)
	if err != nil {
		// File not found → serve index.html for SPA routing.
		s.serveIndex(c)
		return
	}
	defer func() { _ = f.Close() }()

	// Check if it's a directory (e.g. /assets/) – serve index.html instead.
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		s.serveIndex(c)
		return
	}

	// Set cache headers: hashed assets get long cache, index.html gets no-cache.
	if strings.HasPrefix(reqPath, "assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}

	// Detect content type from extension.
	ct := contentTypeByExt(reqPath)
	c.Header("Content-Type", ct)
	c.Status(http.StatusOK)

	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), rs)
	} else {
		_, _ = io.Copy(c.Writer, f)
	}
}

func (s *Server) serveIndex(c *gin.Context) {
	if s.webFS == nil {
		c.String(http.StatusNotFound, "frontend not built")
		return
	}

	f, err := s.webFS.Open("index.html")
	if err != nil {
		c.String(http.StatusNotFound, "index.html not found in embedded assets")
		return
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		s.logger.Warn().Err(err).Msg("stat index.html failed")
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)

	if rs, ok := f.(io.ReadSeeker); ok && stat != nil {
		http.ServeContent(c.Writer, c.Request, "index.html", stat.ModTime(), rs)
	} else {
		_, _ = io.Copy(c.Writer, f)
	}
}

func contentTypeByExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	case ".map":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
