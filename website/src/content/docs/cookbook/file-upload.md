---
title: File Upload
description: Accept multipart uploads and save them safely with Zinc.
---

Accept a file from a multipart form and save it under a server-controlled name. Test it with `curl -F "document=@report.pdf" http://localhost:8080/upload`.

```go
package main

import (
	"log"
	"path/filepath"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()

	app.Post("/upload", func(c *zinc.Context) error {
		file, err := c.FormFile("document")
		if err != nil {
			return zinc.NewError(zinc.StatusBadRequest).WithCause(err)
		}

		name := filepath.Base(file.Filename)
		if err := c.SaveFile(file, filepath.Join("uploads", name)); err != nil {
			return err
		}

		return c.Status(zinc.StatusCreated).JSON(zinc.Map{
			"name": name,
			"size": file.Size,
		})
	})

	log.Fatal(app.Listen())
}
```

`SaveFile` creates missing parent directories.

:::danger[Uploads are untrusted input]
Validate size, extension, detected content type, and authorization before keeping an uploaded file. A client-provided filename or content type is not a trustworthy content check.
:::
