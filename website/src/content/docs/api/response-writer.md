---
title: Response Writer
description: Reference for zinc.WrapResponseWriter, which lets middleware observe the status and size of a response.
---

Middleware sometimes needs to know what a handler sent: the status for metrics, or the byte count for logs. `zinc.WrapResponseWriter` wraps the current writer and records both.

```go
func metrics(c *zinc.Context) error {
	base := c.Writer()
	rw := zinc.WrapResponseWriter(base)
	c.SetWriter(rw)
	defer c.SetWriter(base) // always restore the original writer

	start := time.Now()
	err := c.Next()

	requestDuration.
		WithLabelValues(c.FullPath(), strconv.Itoa(rw.Status())).
		Observe(time.Since(start).Seconds())
	return err
}
```

## Interface

```go
type ResponseWriter interface {
	http.ResponseWriter
	Status() int       // 200 until another status is written
	BytesWritten() int // body bytes written so far
	Written() bool     // whether headers or body have been sent
}
```

## Errors returned by the chain

When a handler returns an error, the error handler writes the response after your middleware has already returned, so `rw.Status()` still reports `200`. To measure the final response, send the error to the error handler first:

```go
err := c.Next()
if err != nil {
	c.HandleError(err) // writes the error response through rw now
}
record(rw.Status(), rw.BytesWritten())
return err
```

The [Custom Middleware](/cookbook/middleware/) recipe uses this pattern in a complete program.

## Optional interfaces

The wrapper keeps the optional interfaces of the writer it wraps: `http.Flusher`, `http.Hijacker`, `io.ReaderFrom`, and `http.Pusher`. It also implements `Unwrap() http.ResponseWriter`, so `http.ResponseController` reaches the original writer.
