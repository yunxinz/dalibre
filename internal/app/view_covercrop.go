package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"dalibre/cmd/dalibre/locales"
	"dalibre/internal/app/icons"
	"dalibre/internal/app/theme"
	"dalibre/internal/app/widgets"
	"dalibre/internal/calibre"
)

// cropDialog is the Edit metadata dialog's crop-cover step.
func (a *App) cropDialog(c *ui.Context, b *calibre.Book) {
	cp := &a.crop
	widgets.DialogFrame(c, &cp.open, 560, func() {
		ui.Text(c, locales.CoverCropTitle()).FontSize(16).FontWeight(600)
	}, func() {
		ui.Column(c).Padding(theme.Space24).Gap(theme.Space16).AlignItems(ui.Stretch).Children(func() {
			a.cropViewport(c)
			a.cropPresetsRow(c)
			a.cropReadout(c)
		})
	}, func() {
		ui.Spacer(c)
		if widgets.Button(c, widgets.Secondary, nil, locales.CoverCropReset()).Clicked() {
			a.cropReset()
		}
		if widgets.Button(c, widgets.Secondary, nil, locales.CommonCancel()).Clicked() {
			cp.open = false
		}
		if widgets.Button(c, widgets.Primary, icons.Crop, locales.CoverCropApply()).Clicked() {
			a.cropApply()
		}
	})
}

// cropViewport is the source image with the dim, guides and handles over
// it, all placed by the contain-fit of the image in the viewport.
func (a *App) cropViewport(c *ui.Context) {
	cp := &a.crop
	ui.Box(c).FillWidth().Height(380).Radius(theme.RadiusLG).Background(pal().Subtle).Clip().
		Bind(&cp.box).Children(func() {
		if cp.img == nil {
			ui.Box(c).Fill().Center().Children(func() {
				ui.Text(c, locales.CoverCropLoading()).FontSize(13).TextColor(pal().TextTertiary)
			})
			return
		}
		vw, vh := cp.box.Bounds(c).W, cp.box.Bounds(c).H
		if vw <= 0 || vh <= 0 {
			c.Invalidate() // laid out after this build; the next places the crop
			return
		}
		b := cp.img.Bounds()
		sw, sh := float32(b.Dx()), float32(b.Dy())
		s := min(vw/sw, vh/sh)
		cp.fit = frect{(vw - sw*s) / 2, (vh - sh*s) / 2, (vw + sw*s) / 2, (vh + sh*s) / 2}
		toView := func(r frect) frect {
			return frect{cp.fit.x0 + r.x0*s, cp.fit.y0 + r.y0*s, cp.fit.x0 + r.x1*s, cp.fit.y0 + r.y1*s}
		}
		vr := toView(cp.rect)
		ui.Image(c, cp.bmp).Absolute().Left(cp.fit.x0).Top(cp.fit.y0).Width(cp.fit.w()).Height(cp.fit.h())
		// The dim and guides sit over the image and under the handles.
		ui.Box(c).Absolute().Left(0).Top(0).Width(vw).Height(vh).PassThrough().DrawOver(func(p *ui.Painter, r ui.Rect) {
			dim := ui.Hex("#000000").Alpha(0.45)
			rect := func(x, y, w, h float32) ui.Rect { return ui.Rect{X: r.X + x, Y: r.Y + y, W: w, H: h} }
			p.Fill(rect(0, 0, vw, vr.y0), dim, 0)
			p.Fill(rect(0, vr.y1, vw, vh-vr.y1), dim, 0)
			p.Fill(rect(0, vr.y0, vr.x0, vr.h()), dim, 0)
			p.Fill(rect(vr.x1, vr.y0, vw-vr.x1, vr.h()), dim, 0)
			p.Stroke(rect(vr.x0, vr.y0, vr.w(), vr.h()), pal().SurfaceRaised, 0, 1.5)
			third := ui.Hex("#ffffff").Alpha(0.25)
			for i := 1; i < 3; i++ {
				x, y := vr.x0+vr.w()/3*float32(i), vr.y0+vr.h()/3*float32(i)
				p.Line(r.X+x, r.Y+vr.y0, r.X+x, r.Y+vr.y1, 1, third)
				p.Line(r.X+vr.x0, r.Y+y, r.X+vr.x1, r.Y+y, 1, third)
			}
		})
		m := ui.Box(c).Absolute().Left(vr.x0).Top(vr.y0).Width(vr.w()).Height(vr.h()).
			Cursor(ui.CursorMove).Label("crop area")
		if dx, dy, ok := m.Dragged(); ok && (dx != 0 || dy != 0) {
			cp.rect = moveCrop(cp.rect, dx/s, dy/s, sw, sh)
		}
		// Later handles sit over earlier ones where they overlap, so the
		// move area gives way to the edges, they to the corners.
		a.cropEdgeHandles(c, vr, s, sw, sh)
		a.cropCornerHandles(c, vr, s, sw, sh)
	})
}

// cropHandle is a handle's hit box, hit wide, with a small visible mark.
func cropHandle(c *ui.Context, label string, cursor ui.Cursor, x, y, w, h float32, mark func()) ui.Element {
	return ui.Box(c).Absolute().Left(x).Top(y).Width(w).Height(h).Cursor(cursor).Label(label).
		Center().Children(func() {
		mark()
	})
}

// cropEdgeHandles places the four edge handles on vr, the crop in view px.
func (a *App) cropEdgeHandles(c *ui.Context, vr frect, s, sw, sh float32) {
	cp := &a.crop
	bar := func(w, h float32) func() {
		return func() {
			ui.Box(c).Size(w, h).Radius(2).Background(pal().SurfaceRaised).Border(1, pal().Border).
				Shadow(0, 1, 2, 0, ui.RGBA(26, 23, 18, 0.08))
		}
	}
	edges := []struct {
		label        string
		verticalDrag bool
		x, y, w, h   float32
		edge         int
	}{
		{"top", true, vr.x0, vr.y0 - 12, vr.w(), 24, 0},
		{"right", false, vr.x1 - 12, vr.y0, 24, vr.h(), 1},
		{"bottom", true, vr.x0, vr.y1 - 12, vr.w(), 24, 2},
		{"left", false, vr.x0 - 12, vr.y0, 24, vr.h(), 3},
	}
	for _, e := range edges {
		cursor := ui.CursorResizeEW
		mark := bar(4, 20)
		if e.verticalDrag {
			cursor, mark = ui.CursorResizeNS, bar(20, 4)
		}
		h := cropHandle(c, e.label, cursor, e.x, e.y, e.w, e.h, mark)
		if dx, dy, ok := h.Dragged(); ok && (dx != 0 || dy != 0) {
			cp.rect = dragEdge(cp.rect, e.edge, dx/s, dy/s, cp.ratio, sw, sh)
		}
	}
}

// cropCornerHandles places the four corner handles on vr, the crop in view px.
func (a *App) cropCornerHandles(c *ui.Context, vr frect, s, sw, sh float32) {
	cp := &a.crop
	dot := func() {
		ui.Box(c).Size(12, 12).Radius(6).Background(pal().SurfaceRaised).Border(1, pal().Border).
			Shadow(0, 1, 2, 0, ui.RGBA(26, 23, 18, 0.08))
	}
	corners := []struct {
		label    string
		diagonal bool // runs top-left to bottom-right
		x, y     float32
		corner   int
	}{
		{"top left", true, vr.x0 - 12, vr.y0 - 12, 0},
		{"top right", false, vr.x1 - 12, vr.y0 - 12, 1},
		{"bottom right", true, vr.x1 - 12, vr.y1 - 12, 2},
		{"bottom left", false, vr.x0 - 12, vr.y1 - 12, 3},
	}
	for _, k := range corners {
		cursor := ui.CursorResizeNESW
		if k.diagonal {
			cursor = ui.CursorResizeNWSE
		}
		h := cropHandle(c, k.label, cursor, k.x, k.y, 24, 24, dot)
		if dx, dy, ok := h.Dragged(); ok && (dx != 0 || dy != 0) {
			cp.rect = dragCorner(cp.rect, k.corner, dx/s, dy/s, cp.ratio, sw, sh)
		}
	}
}

// cropPresetsRow is the segmented control of the crop's aspect ratios.
func (a *App) cropPresetsRow(c *ui.Context) {
	cp := &a.crop
	segs := make([]segment, len(cropPresets))
	for i, p := range cropPresets {
		label := p.text
		if p.label != nil {
			label = p.label()
		}
		segs[i] = segment{
			label: label,
			on:    cp.preset == i,
			onClick: func() {
				cp.preset, cp.ratio = i, p.ratio
				if p.ratio > 0 {
					b := cp.img.Bounds()
					cp.rect = fitRatioRect(float32(b.Dx()), float32(b.Dy()), p.ratio)
				}
			},
		}
	}
	segmented(c, segs, true)
}

// cropReadout is the crop's size and ratio, and the source's size.
func (a *App) cropReadout(c *ui.Context) {
	cp := &a.crop
	pw, ph := cropPixels(cp.rect)
	ratio := cp.ratio
	if ratio == 0 {
		ratio = cp.rect.w() / cp.rect.h()
	}
	b := cp.img.Bounds()
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%d × %d px", pw, ph)).Font(theme.FontMono).FontSize(13)
		ui.Text(c, " · "+nearestRatio(ratio)).FontSize(13).TextColor(pal().TextSecondary)
		ui.Spacer(c)
		ui.Text(c, locales.CoverCropSource(b.Dx(), b.Dy())).FontSize(12).TextColor(pal().TextTertiary)
	})
}

// cropReset puts the crop back to the whole image at the chosen ratio.
func (a *App) cropReset() {
	cp := &a.crop
	b := cp.img.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	if cp.ratio > 0 {
		cp.rect = fitRatioRect(w, h, cp.ratio)
	} else {
		cp.rect = frect{0, 0, w, h}
	}
}

// cropApply makes the cropped image the form's new cover, saved with the
// book when the form is.
func (a *App) cropApply() {
	cp := &a.crop
	img, jpg, err := applyCrop(cp.img, cp.rect)
	if err != nil {
		a.toastError(err)
		return
	}
	a.edit.cover, a.edit.coverBmp = jpg, ui.NewBitmap(img)
	cp.open = false
}
