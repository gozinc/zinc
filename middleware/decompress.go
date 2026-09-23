// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/0mjs/zinc"
)

var (
	// Decompression errors distinguish unsupported encodings from corrupt input.
	ErrDecompressUnsupportedEncoding = errors.New("zincdecompress: unsupported content encoding")
	ErrDecompressInvalidBody         = errors.New("zincdecompress: invalid compressed body")
)

// DecompressConfig controls request decompression and expansion limits.
type DecompressConfig struct {
	Skipper             func(*zinc.Context) bool
	MaxDecompressedSize int64
}

// Decompress accepts gzip request bodies without an expansion limit.
func Decompress() zinc.Middleware {
	return DecompressWithConfig(DecompressConfig{})
}

// DecompressWithConfig transparently exposes gzip input to downstream handlers.
// MaxDecompressedSize should be set for untrusted input to bound expansion.
func DecompressWithConfig(config DecompressConfig) zinc.Middleware {
	if config.MaxDecompressedSize < 0 {
		panic("zincdecompress: MaxDecompressedSize must be greater than or equal to zero")
	}

	return func(c *zinc.Context) error {
		if config.Skipper != nil && config.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		if req == nil || req.Body == nil {
			return c.Next()
		}

		encoding := strings.TrimSpace(strings.ToLower(req.Header.Get(zinc.HeaderContentEncoding)))
		if encoding == "" || encoding == "identity" {
			return c.Next()
		}
		if encoding != "gzip" {
			return errors.Join(zinc.ErrUnsupportedMediaType, fmt.Errorf("%w: %s", ErrDecompressUnsupportedEncoding, encoding))
		}

		reader, err := gzip.NewReader(req.Body)
		if err != nil {
			return errors.Join(zinc.ErrBadRequest, fmt.Errorf("%w: %v", ErrDecompressInvalidBody, err))
		}

		bodyReader := io.Reader(reader)
		if config.MaxDecompressedSize > 0 {
			bodyReader = &decompressedLimitReader{
				reader:    reader,
				remaining: config.MaxDecompressedSize,
			}
		}

		req.Body = &gzipRequestBody{
			reader:     bodyReader,
			gzipReader: reader,
			body:       req.Body,
		}
		req.Header.Del(zinc.HeaderContentEncoding)
		req.Header.Del(zinc.HeaderContentLength)
		req.ContentLength = -1

		return c.Next()
	}
}

type gzipRequestBody struct {
	reader     io.Reader
	gzipReader *gzip.Reader
	body       io.Closer
}

func (b *gzipRequestBody) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

func (b *gzipRequestBody) Close() error {
	err := b.gzipReader.Close()
	if closeErr := b.body.Close(); err == nil {
		err = closeErr
	}
	return err
}

type decompressedLimitReader struct {
	reader    io.Reader
	remaining int64
	exceeded  bool
}

func (r *decompressedLimitReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.exceeded {
		return 0, zinc.ErrRequestEntityTooLarge
	}
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			r.exceeded = true
			return 0, zinc.ErrRequestEntityTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:int(r.remaining)]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
