// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"io"
	"io/fs"
	"reflect"
)

// These types are typed-handler outputs that aren't JSON. The output type
// says what the response is, so the handler returns a value and Zinc writes
// it, and the OpenAPI spec documents the media type:
//
//	app.Get("/hello", zinc.Typed(func(c *zinc.Context, _ struct{}) (zinc.Text, error) {
//		return "hello", nil
//	}))
//
// Use Route.Produces when the media type is only known at run time, as with
// Bytes and Stream.

// Text is sent as text/plain.
type Text string

// HTML is sent as text/html.
type HTML string

// Bytes is sent with media type Type, or application/octet-stream when
// Type is empty.
type Bytes struct {
	Type string
	Data []byte
}

// File serves a file from Path on disk, or from FS when FS is set, with the
// semantics of Context.File and Context.FileFS: ranges, conditional requests
// and a Content-Type from the extension. A non-empty Name makes it a
// download with that file name, as Context.Attachment does.
type File struct {
	Path string
	FS   fs.FS
	Name string
}

// Stream copies Reader to the response with media type Type, or
// application/octet-stream when Type is empty. A Reader that's also an
// io.Closer is closed after the copy.
type Stream struct {
	Type   string
	Reader io.Reader
}

// Redirect sends the client to its URL with the route's status, set with
// Route.Status, or 302 Found.
type Redirect string

// outputKind classifies a typed handler's output type.
type outputKind uint8

const (
	outputJSON outputKind = iota
	outputNoContent
	outputText
	outputHTML
	outputBytes
	outputFile
	outputStream
	outputRedirect
)

var outputKinds = map[reflect.Type]outputKind{
	reflect.TypeFor[NoContent](): outputNoContent,
	reflect.TypeFor[Text]():      outputText,
	reflect.TypeFor[HTML]():      outputHTML,
	reflect.TypeFor[Bytes]():     outputBytes,
	reflect.TypeFor[File]():      outputFile,
	reflect.TypeFor[Stream]():    outputStream,
	reflect.TypeFor[Redirect]():  outputRedirect,
}

// kindOf returns the output kind of t: JSON for any type not listed.
func kindOf(t reflect.Type) outputKind {
	return outputKinds[t]
}

// outputWriter returns the writer for Out, chosen once when Typed is called,
// or nil for JSON. A writer for one output type converts to the writer for
// Out only when Out is that type, so each case is exact.
func outputWriter[Out any]() func(*Context, Out) error {
	var w any
	switch kindOf(reflect.TypeFor[Out]()) {
	case outputText:
		w = func(c *Context, v Text) error { return c.String(string(v)) }
	case outputHTML:
		w = func(c *Context, v HTML) error { return c.HTML(string(v)) }
	case outputBytes:
		w = func(c *Context, v Bytes) error { return c.Data(mediaTypeOr(v.Type), v.Data) }
	case outputFile:
		w = func(c *Context, v File) error { return c.serveFile(v.Path, v.FS, v.Name) }
	case outputStream:
		w = func(c *Context, v Stream) error {
			if closer, ok := v.Reader.(io.Closer); ok {
				defer closer.Close()
			}
			return c.Stream(mediaTypeOr(v.Type), v.Reader)
		}
	case outputRedirect:
		w = func(c *Context, v Redirect) error { return c.Redirect(string(v)) }
	default:
		return nil
	}
	return w.(func(*Context, Out) error)
}

func mediaTypeOr(mediaType string) string {
	if mediaType == "" {
		return "application/octet-stream"
	}
	return mediaType
}
