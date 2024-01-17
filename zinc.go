/*
Copyright 2023 - PRESENT Meltred

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package zinc

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

type App struct {
	Server        *echo.Echo
	Addr          string
	TotalHandlers uint32
	Context       context.Context
}

func New() *App {
	server := echo.New()
	server.HideBanner = true
	server.HidePort = true

	return &App{
		Server:  server,
		Context: context.Background(),
	}
}

func (a *App) registerEndpoints() error {
	slog.Info("Registering Handlers...")
	const sourcePath = "./src/pages/"
	const sourcePrefix = "src/pages"

	totalHandlers := uint32(0)

	err := filepath.Walk(sourcePath, func(path string, info fs.FileInfo, err error) error {
		// only file who are generated
		codeFileName, found := strings.CutSuffix(path, "_templ.go")
		if found {
			// remove sourcePath
			endpoint, _ := strings.CutPrefix(codeFileName, sourcePrefix)
			if endpoint == "/index" {
				endpoint = "/"
			}

			// Increasing the count of  total endpoints registered
			// TODO: read the function docs, they say to do something else  which is less error prone
			atomic.AddUint32(&totalHandlers, 1)

			a.Server.GET(endpoint, func(c echo.Context) error {
				// (:0 See path is from line filepath.Walk function above
				html := a.getHTML(path)

				return c.String(http.StatusOK, html)
			})
		}
		return nil
	})

	a.TotalHandlers = totalHandlers
	return err
}

func (a *App) getHTML(filePath string) string {
	return filePath
}

func (a *App) setAddr(baseURL ...string) {
	var url string
	if len(baseURL) == 0 {
		url = ""
	} else {
		url = baseURL[0]
	}

	host, port, err := net.SplitHostPort(url)
	if err != nil {
		slog.Error(err.Error())

		host, port = "127.0.0.1", "7156"
		slog.Info("Using default host and port", "host", host, "port", port)
		if ip := net.ParseIP(strings.Trim(url, "[]")); ip != nil {
			host = ip.String()
		}
	}

	a.Addr = net.JoinHostPort(host, port)
}

func (a *App) Start(baseURL ...string) error {
	a.setAddr(baseURL...)

	err := a.registerEndpoints()
	if err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("Total Handlers registered: %d", a.TotalHandlers))
	slog.Info(fmt.Sprintf("Listening at: %s\n", a.Addr))

	return a.Server.Start(a.Addr)
}
