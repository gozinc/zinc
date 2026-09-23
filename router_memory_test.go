// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"testing"
	"unsafe"
)

func TestRouterMemoryLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout guard is defined for 64-bit targets")
	}

	if size := unsafe.Sizeof(radixNode{}); size > 152 {
		t.Fatalf("radixNode grew to %d bytes; want no more than 152", size)
	}
	if size := unsafe.Sizeof(radixRoute{}); size > 80 {
		t.Fatalf("radixRoute grew to %d bytes; want no more than 80", size)
	}
}
