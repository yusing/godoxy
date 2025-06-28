package notif

import "fmt"

type Color uint

const (
	ColorError   Color = 0xff0000
	ColorSuccess Color = 0x00ff00
	ColorInfo    Color = 0x0000ff
)

func (c Color) HexString() string {
	return fmt.Sprintf("#%x", uint(c))
}

func (c Color) DecString() string {
	return fmt.Sprintf("%d", c)
}

func (c Color) String() string {
	return c.HexString()
}
