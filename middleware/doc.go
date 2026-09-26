// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package middleware is the home of Zinc's standard middleware. Each one is
// its own package with a New function that takes an optional Config:
//
//	app.Use(recover.New(), logger.New(), requestid.New())
//	app.Use(cors.New(cors.Config{AllowOrigins: []string{"https://app.example.com"}}))
//
// Every package here depends only on Zinc and the standard library.
// Middleware with third-party dependencies, such as JWT, lives in
// github.com/0mjs/contrib.
//
// To run a middleware for only some requests, wrap it with zinc.Skip.
package middleware
