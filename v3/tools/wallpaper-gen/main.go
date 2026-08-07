// Command wallpaper-gen renders the Veilbox default desktop wallpaper:
// a deterministic dark vertical gradient PNG. It is a build-time tool;
// the generated file is committed and shipped by the desktop
// experience RPM.
//
// Usage: go run ./tools/wallpaper-gen <out.png>
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	width  = 1920
	height = 1080
)

// stops are the gradient color stops, top to bottom.
var stops = [][3]uint8{
	{22, 27, 34},  // near-black blue
	{30, 39, 52},  // slate
	{41, 55, 72},  // steel blue
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: wallpaper-gen <out.png>")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	segments := float64(len(stops) - 1)
	for y := 0; y < height; y++ {
		f := float64(y) / float64(height-1) * segments
		i := int(f)
		if i >= len(stops)-1 {
			i = len(stops) - 2
		}
		t := f - float64(i)
		a, b := stops[i], stops[i+1]
		c := color.RGBA{lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t), 255}
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
