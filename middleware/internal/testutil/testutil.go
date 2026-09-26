// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package testutil holds helpers shared by middleware tests.
package testutil

import "fmt"

// JSONErrorBody is the default error handler's body for status and message.
func JSONErrorBody(status int, message string) string {
	return fmt.Sprintf(`{"error":{"status":%d,"message":%q}}`+"\n", status, message)
}
