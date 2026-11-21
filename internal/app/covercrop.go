package app

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"

	"github.com/egoist/mygo/ui"

	"dalibre/cmd/dalibre/locales"
	"dalibre/internal/calibre"
)

// Crop cover: a step of the Edit metadata dialog that crops the book's
// cover image to a chosen aspect ratio, as the 3:4 of Kindle device
// covers and calibre's own covers, or the 1:1.6 of Amazon's store covers.
// The crop becomes the form's new cover; nothing is written until the form
// is saved.

// frect is a rectangle in source-image pixels; float so drags stay smooth.
type frect struct{ x0, y0, x1, y1 float32 }

func (r frect) w() float32 { return r.x1 - r.x0 }
func (r frect) h() float32 { return r.y1 - r.y0 }

// coverCrop is the crop editor's state: the source image and the crop
// rectangle in the source's pixels.
type coverCrop struct {
	open   bool
	img    image.Image // decoded source; nil while it loads
	bmp    *ui.Bitmap  // the source as drawn
	rect   frect       // crop in source pixels
	ratio  float32     // locked width/height; 0 for a free crop
	preset int         // crop preset on, as an index into cropPresets
	fit    frect       // where the source sits in the viewport, in view px
	box    ui.Handle   // the viewport, measured to place the crop
}

// cropPreset is an aspect ratio the crop can lock to.
type cropPreset struct {
	label func() string // localized, or nil when the label is a plain ratio string
	text  string        // "3:4", "1:1" — locale-neutral labels
	ratio float32       // width/height; 0 = free
}

var cropPresets = []cropPreset{
	{text: "3:4", ratio: 3.0 / 4.0},   // Kindle device covers, and calibre's own (600×800)
	{text: "1:1.6", ratio: 1.0 / 1.6}, // Amazon store covers (1600×2560)
	{text: "1:1", ratio: 1},
	{label: locales.CoverCropFree, ratio: 0},
}

// minCropPx is the least each side of a crop may be, when the image's own
// side allows it.
const minCropPx = 32

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func roundPx(v float32) int { return int(math.Round(float64(v))) }

// fitRatioRect is the largest ratio-locked rect centered in a w×h image.
func fitRatioRect(w, h, ratio float32) frect {
	if w <= 0 || h <= 0 {
		return frect{}
	}
	cw, ch := w, w
	if ratio > 0 {
		ch = w / ratio
		if ch > h {
			cw, ch = h*ratio, h
		}
	} else {
		ch = h
	}
	return frect{(w - cw) / 2, (h - ch) / 2, (w + cw) / 2, (h + ch) / 2}
}

// clampCrop keeps r inside a w×h image with each side at least minCropPx,
// or half the image's side when that is less.
func clampCrop(r frect, w, h float32) frect {
	mx, my := min(float32(minCropPx), w/2), min(float32(minCropPx), h/2)
	r.x0, r.y0 = max(r.x0, 0), max(r.y0, 0)
	r.x1, r.y1 = min(r.x1, w), min(r.y1, h)
	if r.w() < mx {
		cx := (r.x0 + r.x1) / 2
		r.x0 = min(max(cx-mx/2, 0), w-mx)
		r.x1 = r.x0 + mx
	}
	if r.h() < my {
		cy := (r.y0 + r.y1) / 2
		r.y0 = min(max(cy-my/2, 0), h-my)
		r.y1 = r.y0 + my
	}
	return r
}

// cropCorner is the corner at index i: 0..3 as (x0,y0), (x1,y0), (x1,y1), (x0,y1).
func cropCorner(r frect, i int) (float32, float32) {
	switch i {
	case 0:
		return r.x0, r.y0
	case 1:
		return r.x1, r.y0
	case 2:
		return r.x1, r.y1
	default:
		return r.x0, r.y1
	}
}

// dragCorner moves corner (cropCorner's index) by dx, dy in source pixels;
// the opposite corner stays fixed. With the ratio locked, the axis the
// pointer moved farther along leads and the other follows from the ratio;
// a size the image can't hold from the anchored corner shrinks keeping
// the ratio.
func dragCorner(r frect, corner int, dx, dy, ratio, w, h float32) frect {
	x, y := cropCorner(r, corner)
	ox, oy := cropCorner(r, (corner+2)%4)
	nx, ny := x+dx, y+dy
	if ratio <= 0 {
		return clampCrop(frect{min(nx, ox), min(ny, oy), max(nx, ox), max(ny, oy)}, w, h)
	}
	sw, sh := absf(nx-ox), absf(ny-oy)
	if absf(dx) >= absf(dy) {
		sh = sw / ratio
	} else {
		sw = sh * ratio
	}
	// What the image offers from the anchored corner bounds the size.
	left, up := corner == 0 || corner == 3, corner == 0 || corner == 1
	aw, ah := w-ox, h-oy
	if left {
		aw = ox
	}
	if up {
		ah = oy
	}
	if sw > aw || sh > ah {
		if aw/ratio <= ah {
			sw, sh = aw, aw/ratio
		} else {
			sw, sh = ah*ratio, ah
		}
	}
	// Both sides at least their minimum, when the room allows it.
	if mx, my := min(float32(minCropPx), w/2), min(float32(minCropPx), h/2); sw < mx || sh < my {
		if ww := max(mx, my*ratio); ww <= aw && ww/ratio <= ah {
			sw, sh = ww, ww/ratio
		}
	}
	n := frect{ox, oy, ox, oy}
	if left {
		n.x0 = ox - sw
	} else {
		n.x1 = ox + sw
	}
	if up {
		n.y0 = oy - sh
	} else {
		n.y1 = oy + sh
	}
	return clampCrop(n, w, h)
}

// dragEdge moves edge 0..3 (top, right, bottom, left) by dx, dy in source
// pixels; the opposite edge stays fixed. With the ratio locked, the
// perpendicular span follows the ratio, centered where it was.
func dragEdge(r frect, edge int, dx, dy, ratio, w, h float32) frect {
	n := r
	switch edge {
	case 0:
		n.y0 += dy
	case 1:
		n.x1 += dx
	case 2:
		n.y1 += dy
	case 3:
		n.x0 += dx
	}
	if ratio <= 0 {
		return clampCrop(n, w, h)
	}
	mx, my := min(float32(minCropPx), w/2), min(float32(minCropPx), h/2)
	if edge == 0 || edge == 2 {
		// The height leads; the width follows, centered where it was.
		along := h - r.y0
		if edge == 0 {
			along = r.y1
		}
		cx := (r.x0 + r.x1) / 2
		room := 2 * min(cx, w-cx)
		sh := min(absf(n.h()), along, room/ratio)
		if want := max(my, mx/ratio); sh < want && want <= min(along, room/ratio) {
			sh = want
		}
		sw := sh * ratio
		if edge == 0 {
			return clampCrop(frect{cx - sw/2, r.y1 - sh, cx + sw/2, r.y1}, w, h)
		}
		return clampCrop(frect{cx - sw/2, r.y0, cx + sw/2, r.y0 + sh}, w, h)
	}
	// The width leads; the height follows, centered where it was.
	along := w - r.x0
	if edge == 3 {
		along = r.x0
	}
	cy := (r.y0 + r.y1) / 2
	room := 2 * min(cy, h-cy)
	sw := min(absf(n.w()), along, room*ratio)
	if want := max(mx, my*ratio); sw < want && want <= min(along, room*ratio) {
		sw = want
	}
	sh := sw / ratio
	if edge == 1 {
		return clampCrop(frect{r.x0, cy - sh/2, r.x0 + sw, cy + sh/2}, w, h)
	}
	return clampCrop(frect{r.x1 - sw, cy - sh/2, r.x1, cy + sh/2}, w, h)
}

// moveCrop translates r by dx, dy, clamped inside the image.
func moveCrop(r frect, dx, dy, w, h float32) frect {
	x0 := min(max(r.x0+dx, 0), w-r.w())
	y0 := min(max(r.y0+dy, 0), h-r.h())
	return frect{x0, y0, x0 + r.w(), y0 + r.h()}
}

// nearestRatio writes r as "a:b" reduced, a and b at most 16, when such a
// fraction is within 1% of it; else with two decimals.
func nearestRatio(r float32) string {
	if r <= 0 {
		return ""
	}
	best, bestErr := "", float32(2)
	for a := 1; a <= 16; a++ {
		for b := 1; b <= 16; b++ {
			if err := absf(float32(a)/float32(b)-r) / r; err < 0.01 && err < bestErr {
				g := gcd(a, b)
				best, bestErr = fmt.Sprintf("%d:%d", a/g, b/g), err
			}
		}
	}
	if best != "" {
		return best
	}
	return fmt.Sprintf("%.2f", r)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// cropPixels is the crop's size in pixels, as applyCrop will make it.
func cropPixels(r frect) (int, int) {
	return roundPx(r.x1) - roundPx(r.x0), roundPx(r.y1) - roundPx(r.y0)
}

// applyCrop crops img to r and encodes it as a JPEG, quality 90 as covers
// are written. No resampling: the output's pixels are the crop's.
func applyCrop(img image.Image, r frect) (image.Image, []byte, error) {
	b := img.Bounds()
	x0 := min(max(roundPx(r.x0), b.Min.X), b.Max.X)
	y0 := min(max(roundPx(r.y0), b.Min.Y), b.Max.Y)
	x1 := min(max(roundPx(r.x1), b.Min.X), b.Max.X)
	y1 := min(max(roundPx(r.y1), b.Min.Y), b.Max.Y)
	rect := image.Rect(x0, y0, x1, y1)
	if rect.Dx() < 1 || rect.Dy() < 1 {
		return nil, nil, fmt.Errorf("the crop is empty")
	}
	var out image.Image
	if sub, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	}); ok {
		out = sub.SubImage(rect)
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		xdraw.Draw(dst, dst.Bounds(), img, rect.Min, xdraw.Src)
		out = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 90}); err != nil {
		return nil, nil, err
	}
	return out, buf.Bytes(), nil
}

// openCoverCrop opens the crop editor on the cover the form holds, else
// the library's cover of b.
func (a *App) openCoverCrop(b *calibre.Book) {
	if len(a.edit.cover) > 0 {
		img, _, err := image.Decode(bytes.NewReader(a.edit.cover))
		if err != nil {
			a.toastError(err)
			return
		}
		a.cropOpened(img)
		return
	}
	if b == nil || a.lib == nil {
		return
	}
	path := a.lib.CoverPath(b)
	a.run(func() {
		data, err := os.ReadFile(path)
		var img image.Image
		if err == nil {
			img, _, err = image.Decode(bytes.NewReader(data))
		}
		a.post(func() {
			if err != nil {
				a.toastError(err)
				return
			}
			a.cropOpened(img)
		})
	})
}

// cropOpened shows img in the crop editor, on the first preset.
func (a *App) cropOpened(img image.Image) {
	w, h := float32(img.Bounds().Dx()), float32(img.Bounds().Dy())
	p := cropPresets[0]
	a.crop = coverCrop{open: true, img: img, bmp: ui.NewBitmap(img), ratio: p.ratio, preset: 0,
		rect: fitRatioRect(w, h, p.ratio)}
}
