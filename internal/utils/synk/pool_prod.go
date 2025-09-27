//go:build production

package synk

func addNonPooled(size int) {}
func addDropped(size int)   {}
func addReused(size int)    {}
func initPoolStats()        {}
