//go:build production

package synk

func addNonPooled(size int)                              {}
func addReused(size int)                                 {}
func addGCed(size int)                                   {}
func initPoolStats()                                     {}
func addCleanup(ptr *[]byte, cleanup func(int), arg int) {}
