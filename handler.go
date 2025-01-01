package ungzip

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(ResponseUngzip{})
}

// ResponseUngzip implements an HTTP handler that decompresses gzipped responses
type ResponseUngzip struct {
	// Only process responses from these paths
	Paths []string `json:"paths,omitempty"`

	// Only process responses with these content types
	ContentTypes []string `json:"content_types,omitempty"`

	// Maximum size of response to decompress (in bytes)
	// Default: 10MB
	MaxSize int64 `json:"max_size,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (ResponseUngzip) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.response_ungzip",
		New: func() caddy.Module { return new(ResponseUngzip) },
	}
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (r *ResponseUngzip) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "path":
				if !d.NextArg() {
					return d.ArgErr()
				}
				r.Paths = append(r.Paths, d.Val())
				for d.NextArg() {
					r.Paths = append(r.Paths, d.Val())
				}

			case "content_type":
				if !d.NextArg() {
					return d.ArgErr()
				}
				r.ContentTypes = append(r.ContentTypes, d.Val())
				for d.NextArg() {
					r.ContentTypes = append(r.ContentTypes, d.Val())
				}

			case "max_size":
				if !d.NextArg() {
					return d.ArgErr()
				}
				size, err := strconv.ParseInt(d.Val(), 10, 64)
				if err != nil {
					return d.Errf("invalid max_size: %v", err)
				}
				r.MaxSize = size

			default:
				return d.Errf("unknown subdirective %s", d.Val())
			}
		}
	}

	// Set default max size if not specified
	if r.MaxSize == 0 {
		r.MaxSize = 10 * 1024 * 1024 // 10MB default
	}

	return nil
}

// Provision implements caddy.Provisioner.
func (r *ResponseUngzip) Provision(ctx caddy.Context) error {
	return nil
}

// Validate implements caddy.Validator.
func (r *ResponseUngzip) Validate() error {
	if r.MaxSize < 0 {
		return fmt.Errorf("max_size cannot be negative")
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (r ResponseUngzip) ServeHTTP(w http.ResponseWriter, req *http.Request, next caddyhttp.Handler) error {
	// Check if path matches configured paths
	if len(r.Paths) > 0 {
		matched := false
		for _, path := range r.Paths {
			if strings.HasPrefix(req.URL.Path, path) {
				matched = true
				break
			}
		}
		if !matched {
			return next.ServeHTTP(w, req)
		}
	}

	rec := caddyhttp.NewResponseRecorder(w, nil, nil)
	
	err := next.ServeHTTP(rec, req)
	if err != nil {
		return err
	}

	// Only attempt to decode if the content is gzipped
	if !isGzipped(rec.Header()) {
		return rec.WriteResponse()
	}

	// Check content type if configured
	if len(r.ContentTypes) > 0 {
		contentType := rec.Header().Get("Content-Type")
		matched := false
		for _, ct := range r.ContentTypes {
			if strings.HasPrefix(contentType, ct) {
				matched = true
				break
			}
		}
		if !matched {
			return rec.WriteResponse()
		}
	}

	// Check response size
	if rec.Buffer().Len() > int(r.MaxSize) {
		return rec.WriteResponse()
	}

	body := rec.Buffer().Bytes()
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	// Remove the Content-Encoding header since we're decompressing
	rec.Header().Del("Content-Encoding")
	
	// Update Content-Length for the decompressed content
	rec.Header().Set("Content-Length", strconv.Itoa(len(decompressed)))

	// Replace the response body with decompressed content
	rec.Buffer().Reset()
	rec.Buffer().Write(decompressed)

	return rec.WriteResponse()
}

func isGzipped(header http.Header) bool {
	return strings.Contains(strings.ToLower(header.Get("Content-Encoding")), "gzip")
}

// Interface guards
var (
	_ caddy.Module                = (*ResponseUngzip)(nil)
	_ caddy.Provisioner          = (*ResponseUngzip)(nil)
	_ caddy.Validator            = (*ResponseUngzip)(nil)
	_ caddyhttp.MiddlewareHandler = (*ResponseUngzip)(nil)
	_ caddyfile.Unmarshaler      = (*ResponseUngzip)(nil)
)
