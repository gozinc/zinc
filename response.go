// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// ErrResponseAlreadySent is returned when a handler attempts a second terminal
// response write. SSE is the deliberate exception and supports multiple events.
var ErrResponseAlreadySent = errors.New("response already sent")

const (
	contentType = "Content-Type"
	jsonType    = "application/json; charset=utf-8"
	xmlType     = "application/xml; charset=utf-8"
	yamlType    = "application/yaml; charset=utf-8"
	tomlType    = "application/toml; charset=utf-8"
	plainText   = "text/plain; charset=utf-8"
	htmlType    = "text/html; charset=utf-8"
	eventStream = "text/event-stream"
	octetStream = "application/octet-stream"
)

// SSEvent describes one Server-Sent Events message.
type SSEvent struct {
	Event string
	ID    string
	Retry time.Duration
	Data  any
}

var nullBytes = []byte("null")

var (
	plainTextHeader             = []string{plainText}
	jsonHeader                  = []string{jsonType}
	xmlHeader                   = []string{xmlType}
	htmlHeader                  = []string{htmlType}
	statusNotFoundBytes         = []byte(http.StatusText(http.StatusNotFound))
	statusMethodNotAllowedBytes = []byte(http.StatusText(http.StatusMethodNotAllowed))
)

func bodyAllowed(method string, status int) bool {
	if method == http.MethodHead {
		return false
	}
	if status >= 100 && status < 200 {
		return false
	}
	return status != http.StatusNoContent && status != http.StatusNotModified
}

func (c *Context) responseStatus() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

// SetHeader replaces a response header value.
func (c *Context) SetHeader(key, value string) *Context {
	c.Writer().Header().Set(key, value)
	return c
}

// AppendHeader adds response header values without replacing existing ones.
func (c *Context) AppendHeader(key string, values ...string) *Context {
	for _, value := range values {
		c.Writer().Header().Add(key, value)
	}
	return c
}

// Type sets Content-Type from a filename extension.
func (c *Context) Type(ext string) *Context {
	if ext == "" {
		return c
	}
	if ext[0] != '.' {
		ext = "." + ext
	}
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		c.SetHeader(HeaderContentType, contentType)
	}
	return c
}

// Location sets the response Location header.
func (c *Context) Location(location string) *Context {
	return c.SetHeader(HeaderLocation, location)
}

// Vary appends fields to the response Vary header.
func (c *Context) Vary(fields ...string) *Context {
	return c.AppendHeader(HeaderVary, fields...)
}

// String writes a plain-text response without converting data to []byte.
func (c *Context) String(data string) error {
	if c.written {
		return ErrResponseAlreadySent
	}
	c.written = true

	writer := c.Writer()
	header := writer.Header()
	if len(header[contentType]) == 0 {
		header[contentType] = plainTextHeader
	}

	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	method := ""
	if c.request != nil {
		method = c.request.Method
	}
	if !bodyAllowed(method, status) {
		if status != http.StatusOK {
			writer.WriteHeader(status)
		}
		return nil
	}
	if status != http.StatusOK {
		writer.WriteHeader(status)
	}

	_, err := io.WriteString(writer, data)
	return err
}

// Send chooses a response representation from the dynamic value type.
func (c *Context) Send(data any) error {
	switch value := data.(type) {
	case nil:
		return c.writeResponse(jsonType, func() error {
			_, err := c.Writer().Write(nullBytes)
			return err
		})
	case string:
		return c.String(value)
	case []byte:
		return c.Data(octetStream, value)
	default:
		return c.JSON(value)
	}
}

// Data writes bytes with the supplied content type.
func (c *Context) Data(contentType string, b []byte) error {
	writer, writeBody, err := c.prepareResponse(contentType)
	if err != nil || !writeBody {
		return err
	}
	_, err = writer.Write(b)
	return err
}

// Blob writes bytes with an explicit status and content type.
func (c *Context) Blob(status int, contentType string, b []byte) error {
	return c.Status(status).Data(contentType, b)
}

// JSONBlob writes pre-encoded JSON.
func (c *Context) JSONBlob(status int, b []byte) error {
	return c.Blob(status, jsonType, b)
}

// XMLBlob writes pre-encoded XML.
func (c *Context) XMLBlob(status int, b []byte) error {
	return c.Blob(status, xmlType, b)
}

// HTMLBlob writes pre-encoded HTML.
func (c *Context) HTMLBlob(status int, b []byte) error {
	return c.Blob(status, htmlType, b)
}

// JSON encodes v using the configured JSON codec.
func (c *Context) JSON(v any) error {
	return c.writeJSON(v, "")
}

// JSONPretty encodes v using the configured JSON codec and indentation.
func (c *Context) JSONPretty(v any, indent string) error {
	return c.writeJSON(v, indent)
}

func (c *Context) writeJSON(v any, indent string) error {
	writer, writeBody, err := c.prepareResponse(jsonType)
	if err != nil || !writeBody {
		return err
	}

	if v == nil {
		_, err = writer.Write(nullBytes)
		return err
	}

	if err := c.app.config.JSONCodec.Encode(writer, v, indent); err != nil {
		return err
	}
	return nil
}

// XML encodes v as XML.
func (c *Context) XML(v any) error {
	if v == nil {
		return c.writeResponse(xmlType, func() error {
			_, err := c.Writer().Write(nullBytes)
			return err
		})
	}

	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	return c.writeResponse(xmlType, func() error {
		_, err := c.Writer().Write(buf.Bytes())
		return err
	})
}

// YAML encodes v as YAML.
func (c *Context) YAML(v any) error {
	if v == nil {
		return c.writeResponse(yamlType, func() error {
			_, err := c.Writer().Write(nullBytes)
			return err
		})
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	if err := encoder.Encode(v); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return c.writeResponse(yamlType, func() error {
		_, err := c.Writer().Write(buf.Bytes())
		return err
	})
}

// TOML encodes v as TOML.
func (c *Context) TOML(v any) error {
	if v == nil {
		return c.writeResponse(tomlType, func() error {
			_, err := c.Writer().Write(nullBytes)
			return err
		})
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	return c.writeResponse(tomlType, func() error {
		_, err := c.Writer().Write(buf.Bytes())
		return err
	})
}

// HTML writes an HTML response.
func (c *Context) HTML(data string) error {
	return c.writeResponse(htmlType, func() error {
		_, err := io.WriteString(c.Writer(), data)
		return err
	})
}

// Stream copies r to the response without buffering it in Zinc.
func (c *Context) Stream(contentType string, r io.Reader) error {
	return c.writeResponse(contentType, func() error {
		_, err := io.Copy(c.Writer(), r)
		return err
	})
}

// SSE writes one event and keeps the event-stream response open for more calls.
func (c *Context) SSE(event SSEvent) error {
	writer, writeBody, err := c.prepareSSE()
	if err != nil || !writeBody {
		return err
	}
	return c.writeSSEEvent(writer, event)
}

// Accepts returns the best offered type allowed by the request Accept header.
func (c *Context) Accepts(types ...string) string {
	if len(types) == 0 {
		return ""
	}
	header := c.GetHeader(HeaderAccept)
	if strings.TrimSpace(header) == "" {
		return types[0]
	}

	offers := make([]acceptOffer, 0, len(types))
	for _, offer := range types {
		if strings.TrimSpace(offer) == "" {
			continue
		}
		offers = append(offers, acceptOffer{raw: offer, mediaType: mediaTypeOnly(offer)})
	}
	if len(offers) == 0 {
		return ""
	}

	ranges := parseAcceptHeader(header)
	for _, accept := range ranges {
		for _, offer := range offers {
			if acceptMatches(accept.mediaType, offer.mediaType) {
				return offer.raw
			}
		}
	}
	return ""
}

// Negotiate selects an offered representation or returns ErrNotAcceptable.
func (c *Context) Negotiate(status int, offers map[string]any) error {
	if len(offers) == 0 {
		return ErrNotAcceptable
	}
	types := make([]string, 0, len(offers))
	for contentType := range offers {
		types = append(types, contentType)
	}
	sort.Strings(types)

	selected := c.Accepts(types...)
	if selected == "" {
		return ErrNotAcceptable
	}
	c.Status(status)
	return c.writeNegotiated(selected, offers[selected])
}

// NoContent writes the selected status, defaulting to 204.
func (c *Context) NoContent() error {
	if c.status == 0 || c.status == http.StatusOK {
		c.status = http.StatusNoContent
	}
	_, _, err := c.prepareResponse("")
	return err
}

func (c *Context) writeDefaultErrorResponse(status int, allowHeader string) error {
	if c.written {
		return ErrResponseAlreadySent
	}
	c.written = true

	writer := c.Writer()
	header := writer.Header()
	if len(allowHeader) != 0 {
		header.Set(HeaderAllow, allowHeader)
	}
	if len(header[contentType]) == 0 {
		header[contentType] = plainTextHeader
	}
	if !bodyAllowed(c.Method(), status) {
		writer.WriteHeader(status)
		return nil
	}

	writer.WriteHeader(status)
	switch status {
	case http.StatusNotFound:
		_, err := writer.Write(statusNotFoundBytes)
		return err
	case http.StatusMethodNotAllowed:
		_, err := writer.Write(statusMethodNotAllowedBytes)
		return err
	default:
		_, err := io.WriteString(writer, http.StatusText(status))
		return err
	}
}

// Redirect writes a redirect response, defaulting code to 302.
func (c *Context) Redirect(code int, location string) error {
	if code == 0 {
		code = http.StatusFound
	}
	if c.written {
		return ErrResponseAlreadySent
	}
	c.written = true
	c.Location(location)
	c.Writer().WriteHeader(code)
	return nil
}

// File serves a file from the operating-system filesystem.
func (c *Context) File(filePath string) error {
	return c.serveFile(filePath, nil, "")
}

// FileFS serves a file from filesystem using fs.ValidPath semantics.
func (c *Context) FileFS(filePath string, filesystem fs.FS) error {
	return c.serveFile(filePath, filesystem, "")
}

// Attachment serves a file with an attachment Content-Disposition.
func (c *Context) Attachment(filePath string, name ...string) error {
	downloadName := filepath.Base(filePath)
	if len(name) > 0 && name[0] != "" {
		downloadName = name[0]
	}
	return c.serveFile(filePath, nil, downloadName)
}

// Download is an alias for Attachment.
func (c *Context) Download(filePath string, name ...string) error {
	return c.Attachment(filePath, name...)
}

// Inline serves a file with an inline Content-Disposition.
func (c *Context) Inline(filePath string, name ...string) error {
	inlineName := filepath.Base(filePath)
	if len(name) > 0 && name[0] != "" {
		inlineName = name[0]
	}
	return c.serveFileWithDisposition(filePath, nil, "inline", inlineName)
}

// Render executes the configured renderer and writes the result as HTML.
func (c *Context) Render(name string, data any) error {
	if c.app == nil || c.app.config.Renderer == nil {
		return errors.New("renderer is not configured")
	}
	var buf bytes.Buffer
	if err := c.app.config.Renderer.Render(&buf, name, data, c); err != nil {
		return err
	}
	return c.writeResponse(htmlType, func() error {
		_, err := c.Writer().Write(buf.Bytes())
		return err
	})
}

// SetCookie adds cookie to the response.
func (c *Context) SetCookie(cookie *http.Cookie) {
	c.writeCookie(cookie)
}

// SetSameSite supplies a default SameSite mode for subsequently written cookies.
func (c *Context) SetSameSite(mode http.SameSite) *Context {
	c.sameSite = mode
	return c
}

// ClearCookie expires each named cookie at the root path.
func (c *Context) ClearCookie(names ...string) {
	expires := time.Unix(1, 0).UTC()
	for _, name := range names {
		c.writeCookie(&http.Cookie{
			Name:    name,
			Value:   "",
			Path:    "/",
			MaxAge:  -1,
			Expires: expires,
		})
	}
}

func (c *Context) writeCookie(cookie *http.Cookie) {
	if c.sameSite != 0 && cookie != nil && cookie.SameSite == 0 {
		clone := *cookie
		clone.SameSite = c.sameSite
		cookie = &clone
	}
	http.SetCookie(c.Writer(), cookie)
}

func (c *Context) writeResponse(ct string, writeBody func() error) error {
	if c.written {
		return ErrResponseAlreadySent
	}
	c.written = true
	if ct != "" && c.Writer().Header().Get(contentType) == "" {
		c.Writer().Header().Set(contentType, ct)
	}
	status := c.responseStatus()
	if !bodyAllowed(c.Method(), status) {
		if status != http.StatusOK {
			c.Writer().WriteHeader(status)
		}
		return nil
	}
	if status != http.StatusOK {
		c.Writer().WriteHeader(status)
	}
	if writeBody != nil {
		return writeBody()
	}
	return nil
}

func (c *Context) prepareResponse(ct string) (http.ResponseWriter, bool, error) {
	if c.written {
		return nil, false, ErrResponseAlreadySent
	}
	c.written = true

	writer := c.Writer()
	if ct != "" {
		header := writer.Header()
		if len(header[contentType]) == 0 {
			switch ct {
			case plainText:
				header[contentType] = plainTextHeader
			case jsonType:
				header[contentType] = jsonHeader
			case xmlType:
				header[contentType] = xmlHeader
			case htmlType:
				header[contentType] = htmlHeader
			default:
				header.Set(contentType, ct)
			}
		}
	}

	status := c.responseStatus()
	method := ""
	if c.request != nil {
		method = c.request.Method
	}
	if !bodyAllowed(method, status) {
		if status != http.StatusOK {
			writer.WriteHeader(status)
		}
		return writer, false, nil
	}

	if status != http.StatusOK {
		writer.WriteHeader(status)
	}
	return writer, true, nil
}

func (c *Context) prepareSSE() (http.ResponseWriter, bool, error) {
	writer := c.Writer()
	if c.written {
		if mediaTypeOnly(writer.Header().Get(contentType)) != eventStream {
			return nil, false, ErrResponseAlreadySent
		}
		return writer, bodyAllowed(c.Method(), c.responseStatus()), nil
	}

	c.written = true
	header := writer.Header()
	if len(header[contentType]) == 0 {
		header.Set(contentType, eventStream)
	}
	status := c.responseStatus()
	if !bodyAllowed(c.Method(), status) {
		if status != http.StatusOK {
			writer.WriteHeader(status)
		}
		return writer, false, nil
	}
	if status != http.StatusOK {
		writer.WriteHeader(status)
	}
	return writer, true, nil
}

func (c *Context) writeSSEEvent(w io.Writer, event SSEvent) error {
	if event.Event != "" {
		if err := writeSSEField(w, "event", event.Event); err != nil {
			return err
		}
	}
	if event.ID != "" {
		if err := writeSSEField(w, "id", event.ID); err != nil {
			return err
		}
	}
	if event.Retry > 0 {
		if _, err := fmt.Fprintf(w, "retry: %d\n", event.Retry.Milliseconds()); err != nil {
			return err
		}
	}

	data, err := c.sseData(event.Data)
	if err != nil {
		return err
	}
	if err := writeSSEField(w, "data", string(data)); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

func (c *Context) sseData(data any) ([]byte, error) {
	switch value := data.(type) {
	case nil:
		return nullBytes, nil
	case string:
		return []byte(value), nil
	case []byte:
		return value, nil
	default:
		var buf bytes.Buffer
		codec := JSONCodec(defaultJSONCodec{})
		if c.app != nil && c.app.config.JSONCodec != nil {
			codec = c.app.config.JSONCodec
		}
		if err := codec.Encode(&buf, value, ""); err != nil {
			return nil, err
		}
		return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
	}
}

func writeSSEField(w io.Writer, field, value string) error {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	for _, line := range strings.Split(value, "\n") {
		if _, err := fmt.Fprintf(w, "%s: %s\n", field, line); err != nil {
			return err
		}
	}
	return nil
}

type acceptOffer struct {
	raw       string
	mediaType string
}

type acceptRange struct {
	mediaType   string
	q           float64
	specificity int
	index       int
}

func parseAcceptHeader(header string) []acceptRange {
	parts := strings.Split(header, ",")
	ranges := make([]acceptRange, 0, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		mediaType, params, err := mime.ParseMediaType(part)
		if err != nil {
			mediaType = mediaTypeOnly(part)
			params = nil
		} else {
			mediaType = strings.ToLower(mediaType)
		}
		if mediaType == "" {
			continue
		}

		q := 1.0
		if rawQ := params["q"]; rawQ != "" {
			parsed, err := strconv.ParseFloat(rawQ, 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		if q <= 0 {
			continue
		}

		ranges = append(ranges, acceptRange{
			mediaType:   mediaType,
			q:           q,
			specificity: acceptSpecificity(mediaType),
			index:       index,
		})
	}

	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].q != ranges[j].q {
			return ranges[i].q > ranges[j].q
		}
		if ranges[i].specificity != ranges[j].specificity {
			return ranges[i].specificity > ranges[j].specificity
		}
		return ranges[i].index < ranges[j].index
	})
	return ranges
}

func acceptSpecificity(mediaType string) int {
	switch {
	case mediaType == "*/*":
		return 0
	case strings.HasSuffix(mediaType, "/*"):
		return 1
	default:
		return 2
	}
}

func acceptMatches(accept, offer string) bool {
	if accept == "*/*" {
		return true
	}
	acceptType, acceptSubType, ok := strings.Cut(accept, "/")
	if !ok {
		return false
	}
	offerType, offerSubType, ok := strings.Cut(offer, "/")
	if !ok {
		return false
	}
	if acceptSubType == "*" {
		return acceptType == offerType
	}
	return acceptType == offerType && acceptSubType == offerSubType
}

func (c *Context) writeNegotiated(ct string, value any) error {
	switch mediaTypeOnly(ct) {
	case "application/json":
		if data, ok := value.([]byte); ok {
			return c.Data(jsonType, data)
		}
		return c.JSON(value)
	case "application/xml", "text/xml":
		if data, ok := value.([]byte); ok {
			return c.Data(xmlType, data)
		}
		return c.XML(value)
	case "application/yaml", "application/x-yaml", "text/yaml":
		if data, ok := value.([]byte); ok {
			return c.Data(yamlType, data)
		}
		return c.YAML(value)
	case "application/toml":
		if data, ok := value.([]byte); ok {
			return c.Data(tomlType, data)
		}
		return c.TOML(value)
	case "text/html":
		switch data := value.(type) {
		case []byte:
			return c.Data(htmlType, data)
		case string:
			return c.HTML(data)
		default:
			return c.HTML(fmt.Sprint(data))
		}
	case "text/plain":
		switch data := value.(type) {
		case []byte:
			return c.Data(plainText, data)
		case string:
			return c.String(data)
		default:
			return c.String(fmt.Sprint(data))
		}
	default:
		switch data := value.(type) {
		case []byte:
			return c.Data(ct, data)
		case string:
			return c.Data(ct, []byte(data))
		case io.Reader:
			return c.Stream(ct, data)
		default:
			return c.Data(ct, []byte(fmt.Sprint(data)))
		}
	}
}

func (c *Context) serveFile(filePath string, filesystem fs.FS, downloadName string) error {
	disposition := ""
	if downloadName != "" {
		disposition = "attachment"
	}
	return c.serveFileWithDisposition(filePath, filesystem, disposition, downloadName)
}

func (c *Context) serveFileWithDisposition(filePath string, filesystem fs.FS, disposition, name string) error {
	if c.written {
		return ErrResponseAlreadySent
	}
	c.written = true
	if name != "" && disposition != "" {
		c.SetHeader(HeaderContentDisposition, fmt.Sprintf("%s; filename=%q", disposition, name))
	}

	if filesystem == nil {
		http.ServeFile(c.Writer(), c.Request(), filePath)
		return nil
	}

	file, err := filesystem.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return fs.ErrInvalid
	}

	if rs, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer(), c.Request(), stat.Name(), stat.ModTime(), rs)
		return nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(data)
	http.ServeContent(c.Writer(), c.Request(), stat.Name(), stat.ModTime(), reader)
	return nil
}
