// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package decompress

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

var (
	// Decompression errors distinguish unsupported encodings from corrupt input.
	ErrUnsupportedEncoding = errors.New("decompress: unsupported content encoding")
	ErrInvalidBody         = errors.New("decompress: invalid compressed body")
)

// Config controls request decompression and expansion limits.
type Config struct {
	MaxDecompressedSize int64
}

// New transparently exposes gzip input to downstream handlers.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("decompress", configs)
	if config.MaxDecompressedSize < 0 {
		panic("decompress: MaxDecompressedSize must be greater than or equal to zero")
	}

	return func(c *zinc.Context) error {
		req := c.Request()
		if req == nil || req.Body == nil {
			return c.Next()
		}

		encoding := strings.TrimSpace(strings.ToLower(req.Header.Get(zinc.HeaderContentEncoding)))
		if encoding == "" || encoding == "identity" {
			return c.Next()
		}
		if encoding != "gzip" {
			return errors.Join(zinc.ErrUnsupportedMediaType, fmt.Errorf("%w: %s", ErrUnsupportedEncoding, encoding))
		}

		reader, err := gzip.NewReader(req.Body)
		if err != nil {
			return errors.Join(zinc.ErrBadRequest, fmt.Errorf("%w: %v", ErrInvalidBody, err))
		}

		limit := config.MaxDecompressedSize
		if limit == 0 {
			limit = c.BodyLimit()
		}
		if limit <= 0 {
			limit = 4 << 20
		}
		bodyReader := io.Reader(reader)
		if limit > 0 {
			bodyReader = &decompressedLimitReader{
				reader:    reader,
				remaining: limit,
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
