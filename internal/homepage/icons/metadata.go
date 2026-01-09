package icons

type Meta struct {
	SVG         bool   `json:"SVG"`
	PNG         bool   `json:"PNG"`
	WebP        bool   `json:"WebP"`
	Light       bool   `json:"Light"`
	Dark        bool   `json:"Dark"`
	DisplayName string `json:"-"`
	Tag         string `json:"-"`
}

func (icon *Meta) Filenames(ref string) []string {
	filenames := make([]string, 0)
	if icon.SVG {
		filenames = append(filenames, ref+".svg")
		if icon.Light {
			filenames = append(filenames, ref+"-light.svg")
		}
		if icon.Dark {
			filenames = append(filenames, ref+"-dark.svg")
		}
	}
	if icon.PNG {
		filenames = append(filenames, ref+".png")
		if icon.Light {
			filenames = append(filenames, ref+"-light.png")
		}
		if icon.Dark {
			filenames = append(filenames, ref+"-dark.png")
		}
	}
	if icon.WebP {
		filenames = append(filenames, ref+".webp")
		if icon.Light {
			filenames = append(filenames, ref+"-light.webp")
		}
		if icon.Dark {
			filenames = append(filenames, ref+"-dark.webp")
		}
	}
	return filenames
}
