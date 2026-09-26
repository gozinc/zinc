// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"fmt"
	"strings"
)

// Decoder reads a request body into v. It has the shape of json.Unmarshal,
// so any library's Unmarshal function can be used directly:
//
//	zinc.Config{Decoders: map[string]zinc.Decoder{"application/yaml": yaml.Unmarshal}}
type Decoder func(data []byte, v any) error

// Encoder turns v into a response body. It has the shape of json.Marshal,
// so any library's Marshal function can be used directly.
type Encoder func(v any) ([]byte, error)

// formMediaTypes are bound field by field, not decoded as a whole body.
var formMediaTypes = map[string]bool{
	"application/x-www-form-urlencoded": true,
	"multipart/form-data":               true,
}

// compileDecoders copies decoders under their base media type, and returns
// nil for none, so an app without custom decoders never consults the map.
func compileDecoders(decoders map[string]Decoder) map[string]Decoder {
	if len(decoders) == 0 {
		return nil
	}
	out := make(map[string]Decoder, len(decoders))
	for mediaType, decode := range decoders {
		key := formatKey("Decoders", mediaType)
		if formMediaTypes[key] {
			panic(fmt.Sprintf("zinc: Decoders can't replace %s; forms are bound field by field", key))
		}
		if decode == nil {
			panic(fmt.Sprintf("zinc: Decoders[%q] is nil", mediaType))
		}
		out[key] = decode
	}
	return out
}

// compileEncoders copies encoders under their base media type, and returns
// nil for none.
func compileEncoders(encoders map[string]Encoder) map[string]Encoder {
	if len(encoders) == 0 {
		return nil
	}
	out := make(map[string]Encoder, len(encoders))
	for mediaType, encode := range encoders {
		key := formatKey("Encoders", mediaType)
		if encode == nil {
			panic(fmt.Sprintf("zinc: Encoders[%q] is nil", mediaType))
		}
		out[key] = encode
	}
	return out
}

// formatKey normalizes a configured media type to the form requests are
// matched against: lowercase, without parameters.
func formatKey(field, mediaType string) string {
	key := mediaTypeOnly(mediaType)
	if key == "" || !strings.Contains(key, "/") {
		panic(fmt.Sprintf("zinc: %s key %q is not a media type", field, mediaType))
	}
	return key
}

// decoderFor returns the app's decoder for mediaType, or nil.
func (c *Context) decoderFor(mediaType string) Decoder {
	if c.app == nil || c.app.decoders == nil {
		return nil
	}
	return c.app.decoders[strings.ToLower(mediaType)]
}

// encoderFor returns the app's encoder for mediaType, or nil.
func (c *Context) encoderFor(mediaType string) Encoder {
	if c.app == nil || c.app.encoders == nil {
		return nil
	}
	return c.app.encoders[mediaType]
}

// errNoEncoder reports a media type the app has no way to write.
var errNoEncoder = errors.New("zinc: no encoder for media type")

// Encode writes v as mediaType, using the Encoder configured for it, or the
// built-in JSON or XML encoding. It returns an error for a media type the
// app has no encoder for.
func (c *Context) Encode(mediaType string, v any) error {
	key := mediaTypeOnly(mediaType)
	if encode := c.encoderFor(key); encode != nil {
		return c.writeEncoded(mediaType, encode, v)
	}
	switch key {
	case "application/json":
		return c.JSON(v)
	case "application/xml", "text/xml":
		return c.XML(v)
	}
	return fmt.Errorf("%w %s", errNoEncoder, key)
}

// writeEncoded writes encode's output as returned, under contentType.
func (c *Context) writeEncoded(contentType string, encode Encoder, v any) error {
	data, err := encode(v)
	if err != nil {
		return err
	}
	return c.Data(contentType, data)
}

// decodeBody decodes the cached body with a configured decoder. A decoder's
// error is the client's: a 400, unless it carries its own status.
func decodeBody(c *Context, decode Decoder, v any, requireBody bool) error {
	body, readErr := readRequiredBody(c, requireBody)
	if readErr != nil {
		return wrapBindError("body", readErr)
	}
	if len(body) == 0 {
		return c.Validate(v)
	}
	if err := decode(body, v); err != nil {
		if coder, _ := findStatusCoder(err); coder != nil {
			return err
		}
		return wrapBindError("body", err)
	}
	return c.Validate(v)
}
