package captcha

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/color"
	"image/png"
	"math/big"
	"strings"
)

// 简单像素字体 5x7, 数字0-9
var font5x7 = map[rune][]string{
	'0': {" ## ", "#  #", "#  #", "#  #", "#  #", "#  #", " ## "},
	'1': {"  # ", " ## ", "  # ", "  # ", "  # ", "  # ", " ###"},
	'2': {" ## ", "#  #", "   #", "  # ", " #  ", "#   ", "####"},
	'3': {" ## ", "#  #", "   #", "  # ", "   #", "#  #", " ## "},
	'4': {"#  #", "#  #", "#  #", "####", "   #", "   #", "   #"},
	'5': {"####", "#   ", "#   ", "### ", "   #", "#  #", " ## "},
	'6': {" ## ", "#  #", "#   ", "### ", "#  #", "#  #", " ## "},
	'7': {"####", "   #", "  # ", "  # ", " #  ", " #  ", " #  "},
	'8': {" ## ", "#  #", "#  #", " ## ", "#  #", "#  #", " ## "},
	'9': {" ## ", "#  #", "#  #", " ###", "   #", "   #", " ## "},
}

func drawPixelChar(img *image.RGBA, ch rune, x, y int, c color.RGBA) {
	bitmap, ok := font5x7[ch]
	if !ok {
		return
	}
	for row, line := range bitmap {
		for col, pixel := range line {
			if pixel == '#' {
				img.Set(x+col, y+row, c)
			}
		}
	}
}

func GenerateImage(code string) ([]byte, error) {
	width := 120
	height := 40
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	bgColor := color.RGBA{R: 240, G: 240, B: 240, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, bgColor)
		}
	}

	for i := 0; i < 20; i++ {
		x1, _ := rand.Int(rand.Reader, big.NewInt(int64(width)))
		y1, _ := rand.Int(rand.Reader, big.NewInt(int64(height)))
		x2, _ := rand.Int(rand.Reader, big.NewInt(int64(width)))
		y2, _ := rand.Int(rand.Reader, big.NewInt(int64(height)))
		lineColor := color.RGBA{R: 180, G: 180, B: 180, A: 255}
		drawLine(img, int(x1.Int64()), int(y1.Int64()), int(x2.Int64()), int(y2.Int64()), lineColor)
	}

	for i := 0; i < 100; i++ {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(width)))
		y, _ := rand.Int(rand.Reader, big.NewInt(int64(height)))
		img.Set(int(x.Int64()), int(y.Int64()), color.RGBA{R: 150, G: 150, B: 150, A: 255})
	}

	textColors := []color.RGBA{
		{R: 50, G: 50, B: 180, A: 255},
		{R: 180, G: 30, B: 30, A: 255},
		{R: 20, G: 120, B: 20, A: 255},
		{R: 180, G: 100, B: 20, A: 255},
	}

	chars := strings.Split(code, "")
	charWidth := width / (len(chars) + 1)
	for i, ch := range chars {
		x := charWidth*(i+1) - 2
		offset, _ := rand.Int(rand.Reader, big.NewInt(6))
		y := 14 + int(offset.Int64()) - 3
		clr := textColors[i%len(textColors)]
		drawPixelChar(img, []rune(ch)[0], x, y, clr)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawLine(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx := 1
	if x1 > x2 {
		sx = -1
	}
	sy := 1
	if y1 > y2 {
		sy = -1
	}
	err := dx - dy
	for {
		img.Set(x1, y1, c)
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
