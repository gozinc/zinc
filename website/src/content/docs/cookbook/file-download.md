---
title: File Download
description: Send a file as a named download or show it in the browser, without letting the URL pick a path on disk.
---

This program sends CSV reports as downloads and shows a PDF manual in the browser. You'd use it for exports, invoices or any file a user saves. It shows `c.Attachment` and `c.Inline`, and how to keep a URL from choosing which file on disk gets sent.

## Run it

The program reads files from two folders next to `main.go`:

```bash
mkdir -p exports public
printf 'month,total\njan,120\nfeb,95\n' > exports/monthly.csv
cp ~/Downloads/some.pdf public/manual.pdf   # any PDF
```

## The program

```go title="main.go"
package main

import (
	"log"
	"path/filepath"
	"regexp"

	"github.com/0mjs/zinc"
)

// Report ids are letters, digits, "-" and "_", so an id can't name a path.
var reportID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func main() {
	app := zinc.New()

	app.Get("/reports/{id}", func(c *zinc.Context) error {
		id := c.Param("id")
		if !reportID.MatchString(id) {
			return zinc.ErrNotFound
		}

		path := filepath.Join("exports", id+".csv")
		return c.Attachment(path, "report-"+id+".csv")
	})

	app.Get("/manual", func(c *zinc.Context) error {
		return c.Inline("public/manual.pdf", "zinc-manual.pdf")
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

`curl -OJ` saves the file under the name the server suggests:

```bash
curl -OJ http://localhost:8080/reports/monthly
ls
# report-monthly.csv
```

The `Content-Disposition` header carries that name:

```bash
curl -i http://localhost:8080/reports/monthly
# HTTP/1.1 200 OK
# Accept-Ranges: bytes
# Content-Disposition: attachment; filename="report-monthly.csv"
# Content-Length: 27
# Content-Type: text/csv; charset=utf-8
# Last-Modified: Mon, 28 Sep 2026 00:18:31 GMT
#
# month,total
# jan,120
# feb,95
```

`/manual` sends `inline`, so a browser opens the PDF in a tab instead of saving it:

```bash
curl -I http://localhost:8080/manual
# HTTP/1.1 200 OK
# Content-Disposition: inline; filename="zinc-manual.pdf"
# Content-Type: application/pdf
```

A report that doesn't exist, or an id with other characters, is a `404`:

```bash
curl -i http://localhost:8080/reports/yearly
# HTTP/1.1 404 Not Found
#
# {"error":{"status":404,"message":"Not Found"}}

curl -i http://localhost:8080/reports/a.b
# HTTP/1.1 404 Not Found
#
# {"error":{"status":404,"message":"Not Found"}}
```

## How it works

- `reportID` allows only letters, digits, `-` and `_`. An id such as `..` never reaches the filesystem, so the URL can name a report but not a path.
- `filepath.Join("exports", id+".csv")` builds the path from a folder and extension you control.
- `c.Attachment(path, name)` sends the file with `Content-Disposition: attachment`, which tells the browser to save it as `name`. Without a name, it uses the file's own.
- `c.Inline(path, name)` sends `Content-Disposition: inline`, which asks the browser to display the file when it can.
- A missing file comes back as Zinc's `404`. Files are sent with `http.ServeFile`, so `Content-Type`, `Last-Modified` and range requests work without extra code.

Use `c.File(path)` when you want neither header, and `c.FileFS(name, fsys)` to send a file from an `fs.FS` such as an embedded folder.

## Before production

- Check that the user may see the report before sending it. The id says which file; it doesn't say who may have it.
- When ids come from a database, look up the record and build the path from the record, not from the URL.
- Keep downloadable files outside any folder you serve with `app.Static`, so they can't be fetched around your checks.

## See also

- [Responses and Rendering](/guide/responses-and-rendering/): every way to write a response, files included.
- [File Upload](/cookbook/file-upload/): accept files from a form.
- [Embed Resources](/cookbook/embed-resources/): serve files compiled into the binary.
