---
title: File Download
description: Serve files inline or as named browser downloads without accepting arbitrary filesystem paths.
---

Keep the filesystem path under application control. The route parameter should
identify a record or known file—it should not become a path by itself.

```go
package main

import (
	"log"
	"path/filepath"
	"regexp"

	"github.com/0mjs/zinc"
)

var reportID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func main() {
	app := zinc.New()

	app.Get("/reports/{id}", func(c *zinc.Context) error {
		id := c.Param("id")
		if !reportID.MatchString(id) {
			return zinc.BadRequest("invalid report id")
		}

		path := filepath.Join("exports", id+".csv")
		return c.Attachment(path, "report-"+id+".csv")
	})

	app.Get("/manual", func(c *zinc.Context) error {
		return c.Inline("./public/manual.pdf", "zinc-manual.pdf")
	})

	log.Fatal(app.Listen(":8080"))
}
```

Put a CSV such as `exports/monthly.csv` beside the app, then request it:

```bash
curl -OJ http://localhost:8080/reports/monthly
```

`Download` sets an attachment disposition. `Inline` asks the browser to display
supported content. Use `c.File` when no content-disposition header is needed.
