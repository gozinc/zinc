---
title: File Upload
description: Accept a file from a multipart form, save it under a name you choose, and reject bad or oversized uploads.
---

This program accepts a file posted from an HTML form or `curl -F` and saves it in an `uploads/` folder. You'd use it for avatars, attachments or imports. It shows `c.FormFile` and `c.SaveFile`, and how to answer a missing file or an oversized body.

## The program

```go title="main.go"
package main

import (
	"crypto/rand"
	"errors"
	"log"
	"net/http"
	"path/filepath"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New(zinc.Config{BodyLimit: 10 << 20}) // 10 MiB per request

	app.Post("/upload", func(c *zinc.Context) error {
		file, err := c.FormFile("document")
		switch {
		case errors.Is(err, http.ErrMissingFile), errors.Is(err, http.ErrNotMultipart):
			return zinc.UnprocessableEntity("send a multipart form with a document file")
		case err != nil:
			return err // for example, 413 when the body is over BodyLimit
		}

		// Never use the client's filename as a path: pick your own.
		name := rand.Text() + filepath.Ext(file.Filename)
		if err := c.SaveFile(file, filepath.Join("uploads", name)); err != nil {
			return err
		}

		return c.Status(zinc.StatusCreated).JSON(zinc.Map{
			"name":     name,
			"original": file.Filename,
			"size":     file.Size,
		})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Upload a small CSV:

```bash
printf 'month,total\njan,120\nfeb,95\n' > report.csv

curl -i -F "document=@report.csv" http://localhost:8080/upload
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"name":"7UVUAJTNY3U2CCYPAA46ULPL52.csv","original":"report.csv","size":27}

ls uploads
# 7UVUAJTNY3U2CCYPAA46ULPL52.csv
```

Send the file under the wrong field name, or send no form at all, and you get a `422`:

```bash
curl -i -F "other=@report.csv" http://localhost:8080/upload
# HTTP/1.1 422 Unprocessable Entity
#
# {"error":{"status":422,"message":"send a multipart form with a document file"}}
```

A body over the 10 MiB limit is refused with a `413` before it's read:

```bash
head -c 11000000 /dev/zero > big.bin
curl -i -F "document=@big.bin" http://localhost:8080/upload
# HTTP/1.1 413 Request Entity Too Large
#
# {"error":{"status":413,"message":"Request Entity Too Large"}}
```

## How it works

- `zinc.Config{BodyLimit: 10 << 20}` caps every request body at 10 MiB. Without it, the default is 4 MiB.
- `c.FormFile("document")` parses the multipart form and returns the file's header: its name, size and a way to open it.
- `http.ErrMissingFile` and `http.ErrNotMultipart` mean the client sent the wrong thing, so the handler answers `422`. Other errors, such as the `413`, are returned as they are.
- `rand.Text()` makes a random name, so two uploads called `report.pdf` never overwrite each other, and a crafted filename can't point at another folder. The original name is only echoed back.
- `c.SaveFile` copies the upload to disk and creates `uploads/` if it doesn't exist.

## Before production

:::danger[Uploads are untrusted input]
A client chooses the filename, the extension and the `Content-Type`. None of them tell you what the file contains.
:::

- Check who is uploading before you save anything. Put the route behind your auth middleware.
- Allow only the extensions you expect, and check the content with `http.DetectContentType` on the first 512 bytes.
- Store uploads outside any folder you serve with `app.Static`, so an uploaded HTML or script file can't run in your users' browsers.
- Set `BodyLimit` to the largest file you want to accept, plus a little for the form fields.

## Good to know

### Several files in one field

`c.FormFiles("documents")` returns every file sent under that name. It returns `http.ErrMissingFile` when there are none.

### Where the bytes go while parsing

Go keeps up to 32 MiB of a multipart form in memory and writes larger parts to temporary files. With a 10 MiB `BodyLimit`, uploads stay in memory until `SaveFile` writes them out.

## See also

- [Request Data](/guide/request/): forms, query values and headers.
- [Body Limit](/middleware/bodylimit/): a different size limit per route or group.
- [File Download](/cookbook/file-download/): send files back to the browser.
