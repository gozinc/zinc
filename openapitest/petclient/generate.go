// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package petclient is a client oapi-codegen generates from the conformance
// app's spec. It exists to prove a generator can use a spec Zinc writes.
package petclient

//go:generate go tool oapi-codegen -config cfg.yaml ../testdata/conformance.json
