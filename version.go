// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

// Version is the current version of Zinc.
// This should be updated for each release.
const Version = "0.2.1"

// GetVersion returns the current version of Zinc.
func GetVersion() string {
	return Version
}

// GetVersionHeader returns the full version string for HTTP headers.
func GetVersionHeader() string {
	return "Zinc/" + Version
}
