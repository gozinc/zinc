---
title: Static Files
description: Serve directories, single files, and embedded assets safely.
---

Serve a folder of assets with one call. Zinc answers `GET` and `HEAD`, sets content types, supports `Range` and conditional requests through `http.ServeContent`, and rejects paths that try to escape the folder.

```go
app.Static("/assets", "./public")
```

`GET /assets/css/app.css` now serves `./public/css/app.css`. A directory that does not exist is not detected until a request arrives, so check the path at startup if a wrong deploy path should stop the process.

Zinc opens one confined directory handle on the first file request and retains it for later requests. Symlinks remain inside that directory. If you rename or replace the directory after the first request, the mount continues to serve the original directory until the app is recreated. Use `StaticFS` with a filesystem you manage if assets must switch without restarting. `app.Shutdown(ctx)` releases the handle after requests drain. If another `http.Server` serves the app as an `http.Handler`, call `app.Close()` after that server stops to release the handle. `StaticFS` uses the filesystem you supply; you retain ownership of it.

## A single file

```go
app.File("/robots.txt", "./public/robots.txt")
```

## Embedded assets

Compile assets into the binary with `embed`, then serve any `fs.FS`:

```go
//go:embed public
var public embed.FS

func main() {
	app := zinc.New()

	assets, err := fs.Sub(public, "public")
	if err != nil {
		log.Fatal(err)
	}
	app.StaticFS("/assets", assets)
	app.FileFS("/favicon.ico", "favicon.ico", assets)

	log.Fatal(app.Listen(":8080"))
}
```

`fs.Sub` strips the `public/` prefix so URLs do not include it. The [Embed Resources](/cookbook/embed-resources/) recipe has a runnable version.

## Index files and directory listings

```go
app.Static("/docs", "./site",
	zinc.WithStaticIndex("home.html"),  // served for directory requests; default "index.html"
	zinc.WithStaticBrowse(true),        // list files when there is no index
)
```

Directory listing is off by default. Turn it on only for content you intend to publish.

## Existing file servers

A standard `http.FileServer`, or any other handler, can own a path instead:

```go
app.Mount("/files", http.FileServer(http.Dir("./shared"))) // Mount strips "/files" itself
```

Prefer `Static` and `StaticFS` in new code. They reject methods other than `GET` and `HEAD`, never list directories unless asked, and serve an index file by default.

## Static middleware

The [Static middleware](/middleware/static/) serves files from `app.Use` and falls through to your routes when a file does not exist. It suits apps where files and routes share the same paths.

## Next steps

- [Embed Resources](/cookbook/embed-resources/) ships a single binary with its assets.
- [Responses and Rendering](/guide/responses-and-rendering/) sends individual files from handlers.
