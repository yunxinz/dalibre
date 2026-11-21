package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFitRatioRect(t *testing.T) {
	if r := fitRatioRect(600, 900, 0.625); r != (frect{18.75, 0, 581.25, 900}) {
		t.Errorf("portrait 600×900 @ 5:8 = %v", r)
	}
	if r := fitRatioRect(1200, 600, 0.625); r != (frect{412.5, 0, 787.5, 600}) {
		t.Errorf("landscape 1200×600 @ 5:8 = %v", r)
	}
	if r := fitRatioRect(600, 900, 0); r != (frect{0, 0, 600, 900}) {
		t.Errorf("free = %v", r)
	}
}

func TestNearestRatio(t *testing.T) {
	for r, want := range map[float32]string{
		0.625: "5:8", 0.75: "3:4", 1: "1:1", 1.6: "8:5", 0.7417: "0.74",
	} {
		if got := nearestRatio(r); got != want {
			t.Errorf("nearestRatio(%v) = %q, want %q", r, got, want)
		}
	}
}

func TestClampCrop(t *testing.T) {
	r := clampCrop(frect{-40, -10, 700, 950}, 600, 900)
	if r.x0 < 0 || r.y0 < 0 || r.x1 > 600 || r.y1 > 900 {
		t.Errorf("outside the image: %v", r)
	}
	r = clampCrop(frect{100, 100, 104, 104}, 600, 900)
	if r.w() < 32 || r.h() < 32 {
		t.Errorf("below the minimum side: %v", r)
	}
	r = clampCrop(frect{10, 10, 14, 14}, 40, 40)
	if r.w() < 20 || r.h() < 20 {
		t.Errorf("minimum must give way to half the image: %v", r)
	}
}

func TestDragCornerLocked(t *testing.T) {
	r0 := fitRatioRect(600, 900, 0.75)
	// Dragging the bottom-right corner keeps the ratio and the top-left
	// corner, in and out of the image.
	for _, d := range [][2]float32{{-100, -50}, {200, 200}, {-500, -800}, {30, -90}} {
		r := dragCorner(r0, 2, d[0], d[1], 0.75, 600, 900)
		if got := r.w() / r.h(); math.Abs(float64(got-0.75)) > 1e-4 {
			t.Errorf("drag %v: ratio %v, want 0.75 (%v)", d, got, r)
		}
		if r.x0 != r0.x0 || r.y0 != r0.y0 {
			t.Errorf("drag %v: anchored corner moved: %v", d, r)
		}
		if r.x0 < 0 || r.y0 < 0 || r.x1 > 600 || r.y1 > 900 {
			t.Errorf("drag %v: outside the image: %v", d, r)
		}
	}
	// A free drag moves both axes on their own.
	r := dragCorner(frect{0, 0, 600, 900}, 1, -400, 100, 0, 600, 900)
	if r.w() >= 600 || r.h() >= 900 {
		t.Errorf("free drag %v", r)
	}
}

func TestDragEdgeLocked(t *testing.T) {
	r0 := fitRatioRect(600, 900, 0.75)
	for _, d := range [][2]float32{{0, 100}, {0, -300}} {
		r := dragEdge(r0, 2, d[0], d[1], 0.75, 600, 900)
		if got := r.w() / r.h(); math.Abs(float64(got-0.75)) > 1e-4 {
			t.Errorf("drag %v: ratio %v, want 0.75 (%v)", d, got, r)
		}
		if r.y0 != r0.y0 {
			t.Errorf("drag %v: the top edge moved: %v", d, r)
		}
		if c := (r.x0 + r.x1) / 2; c != (r0.x0+r0.x1)/2 {
			t.Errorf("drag %v: width left its center: %v", d, r)
		}
	}
	// Dragging the right edge out of the image shrinks keeping the ratio.
	r := dragEdge(r0, 1, 400, 0, 0.75, 600, 900)
	if r.x1 > 600 {
		t.Errorf("right edge out: %v", r)
	}
	if got := r.w() / r.h(); math.Abs(float64(got-0.75)) > 1e-4 {
		t.Errorf("right edge out: ratio %v, want 0.75", got)
	}
}

func TestMoveCrop(t *testing.T) {
	r := moveCrop(frect{0, 0, 300, 300}, 500, -500, 600, 900)
	if r != (frect{300, 0, 600, 300}) {
		t.Errorf("move = %v", r)
	}
}

// cropTestCover is a 600×900 cover with something to crop in every part.
func cropTestCover(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / 600), uint8(y * 255 / 900), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func absF(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestCropCover(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("Piranesi"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := a.lib.SetCover(a.current().ID, cropTestCover(t)); err != nil {
		t.Fatal(err)
	}
	a.reload(nil)
	tt.Frame()
	a.openEditor(a.current())
	tt.Frame()
	if err := tt.Click("Crop cover"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Crop cover") {
		t.Fatalf("crop dialog not open: %q", tt.Texts())
	}
	if !tt.HasText("600 × 800 px") || !tt.HasText("3:4") {
		t.Fatalf("default readout: %q", tt.Texts())
	}
	if !tt.HasText("Source 600 × 900 px") {
		t.Fatalf("source size missing: %q", tt.Texts())
	}
	shot(t, tt, "crop")

	// Drag the bottom-right corner inward and check the size follows with
	// the ratio held.
	r, ok := tt.Find("bottom right")
	if !ok {
		t.Fatal("no bottom-right handle")
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	tt.Press(cx, cy)
	for i := 1; i <= 4; i++ {
		tt.Move(cx-8*float32(i), cy-14*float32(i))
	}
	tt.Release(cx-32, cy-56)
	tt.Frame()
	if tt.HasText("600 × 800 px") {
		t.Fatalf("drag moved nothing: %q", tt.Texts())
	}
	if !tt.HasText("3:4") {
		t.Fatalf("ratio left the lock: %q", tt.Texts())
	}
	var wantW, wantH int
	for _, s := range tt.Texts() {
		if !strings.HasSuffix(s, " px") || !strings.Contains(s, " × ") {
			continue
		}
		parts := strings.SplitN(strings.TrimSuffix(s, " px"), " × ", 2)
		w, err1 := strconv.Atoi(parts[0])
		h, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil && w != 600 {
			wantW, wantH = w, h
		}
	}
	if wantW == 0 {
		t.Fatalf("no size readout: %q", tt.Texts())
	}
	shot(t, tt, "crop-dragged")

	if err := tt.Click("Crop"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.crop.open {
		t.Error("crop dialog still open")
	}
	if !a.editOpen {
		t.Error("editor closed")
	}
	img, _, err := image.Decode(bytes.NewReader(a.edit.cover))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if math.Abs(float64(b.Dx())/float64(b.Dy())-0.75) > 0.01 {
		t.Errorf("cropped %d×%d, not 3:4", b.Dx(), b.Dy())
	}
	if absF(b.Dx()-wantW) > 1 || absF(b.Dy()-wantH) > 1 {
		t.Errorf("cropped %d×%d, readout said %d×%d", b.Dx(), b.Dy(), wantW, wantH)
	}
	if !tt.HasText("New cover, saved with the book") {
		t.Errorf("no new-cover note: %q", tt.Texts())
	}
}
