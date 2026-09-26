// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

//go:build race

package zinc

// raceEnabled skips allocation comparisons: under -race, sync.Pool drops
// items at random, so pooled objects are sometimes allocated again.
const raceEnabled = true
