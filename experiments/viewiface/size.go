package viewiface

import "unsafe"

// sizeOf reports a value's width. Kept out of the benchmark file so the unsafe
// import is isolated to one place.
func sizeOf[T any](v T) int { return int(unsafe.Sizeof(v)) }
