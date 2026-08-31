package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------- Кастомный виджет с поддержкой левого и правого клика ----------
type ClickableLabel struct {
	widget.BaseWidget
	Label             *widget.Label
	Icon              *canvas.Image
	OnTapped          func()
	OnDoubleTapped    func()
	OnTappedSecondary func(fyne.Position)
	Selected          bool
}

func (c *ClickableLabel) SetSelected(selected bool) {
	c.Selected = selected
	c.Refresh()
}

func (c *ClickableLabel) SetIcon(isDir bool) {
	if isDir {
		c.Icon.Resource = theme.NewPrimaryThemedResource(theme.FolderIcon())
	} else {
		c.Icon.Resource = theme.FileIcon()
	}
	c.Icon.Refresh()
}

func (c *ClickableLabel) Tapped(*fyne.PointEvent) {
	if c.OnTapped != nil {
		c.OnTapped()
	}
}

func (c *ClickableLabel) MouseDown(ev *desktop.MouseEvent) {
	if ev.Modifier&fyne.KeyModifierControl != 0 {
		ctrlHeld = true
	}
}

func (c *ClickableLabel) MouseUp(ev *desktop.MouseEvent) {
	if ev.Modifier&fyne.KeyModifierControl == 0 {
		ctrlHeld = false
	}
}

func (c *ClickableLabel) DoubleTapped(*fyne.PointEvent) {
	if c.OnDoubleTapped != nil {
		c.OnDoubleTapped()
	}
}

func (c *ClickableLabel) TappedSecondary(ev *fyne.PointEvent) {
	if c.OnTappedSecondary != nil {
		c.OnTappedSecondary(ev.AbsolutePosition)
	}
}

func (c *ClickableLabel) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	return &selectableLabelRenderer{c: c, bg: bg, icon: c.Icon, label: c.Label}
}

type selectableLabelRenderer struct {
	c     *ClickableLabel
	bg    *canvas.Rectangle
	icon  *canvas.Image
	label *widget.Label
}

func (r *selectableLabelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.icon, r.label}
}

func (r *selectableLabelRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	iconSize := fyne.NewSize(24, s.Height)
	labelSize := fyne.NewSize(s.Width-iconSize.Width, s.Height)
	r.icon.Resize(iconSize)
	r.icon.Move(fyne.NewPos(4, 0))
	r.label.Resize(labelSize)
	r.label.Move(fyne.NewPos(28, 0))
}

func (r *selectableLabelRenderer) MinSize() fyne.Size {
	return fyne.NewSize(28+r.label.MinSize().Width, r.label.MinSize().Height)
}

func (r *selectableLabelRenderer) Refresh() {
	if r.c.Selected {
		r.bg.FillColor = color.NRGBA{R: 51, G: 153, B: 255, A: 180}
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.bg.Refresh()
	r.icon.Refresh()
	r.label.Refresh()
}

func (r *selectableLabelRenderer) Destroy() {}

func NewClickableLabel(onTap func(), onDoubleTap func(), onTapSecondary func(fyne.Position)) *ClickableLabel {
	label := widget.NewLabel("")
	label.Alignment = fyne.TextAlignLeading
	icon := canvas.NewImageFromResource(theme.FileIcon())
	icon.FillMode = canvas.ImageFillContain
	c := &ClickableLabel{
		Label:             label,
		Icon:              icon,
		OnTapped:          onTap,
		OnDoubleTapped:    onDoubleTap,
		OnTappedSecondary: onTapSecondary,
	}
	c.ExtendBaseWidget(c)
	return c
}
