---
title: Embed Resources
description: Compile static assets into a Zinc binary with embed.FS.
---

Ship a single binary that contains its own HTML, CSS, and images. Go's `embed` package compiles the files in, and `app.StaticFS` serves them. Nothing needs to be copied next to the executable at deploy time.

```go
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/0mjs/zinc"
)

//go:embed public
var assets embed.FS

func main() {
	public, err := fs.Sub(assets, "public")
	if err != nil {
		log.Fatal(err)
	}

	app := zinc.New()
	app.StaticFS("/", public)

	log.Fatal(app.Listen())
}
```

Files under `public/` are now part of the executable. Configure `zinc.WithStaticIndex("index.html")` for a different index filename or `zinc.WithStaticBrowse(true)` to allow directory listings.
