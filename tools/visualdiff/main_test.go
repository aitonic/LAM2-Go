package main

import (
	"image"
	"image/color"
	"testing"
)

func TestExactRenderedScreenshotPasses(t *testing.T) {
	x := image.NewRGBA(image.Rect(0, 0, 10, 10))
	x.Set(2, 2, color.RGBA{R: 220, A: 255})
	r, e := compare(x, x, 16, 0)
	if e != nil || !r.Passed || r.Changed != 0 {
		t.Fatalf("unexpected identical screenshot result: %+v %v", r, e)
	}
}
func TestPixelMismatchRejects(t *testing.T) {
	a, b := image.NewRGBA(image.Rect(0, 0, 10, 10)), image.NewRGBA(image.Rect(0, 0, 10, 10))
	b.Set(1, 1, color.RGBA{B: 255, A: 255})
	r, e := compare(a, b, 16, 0)
	if e != nil || r.Passed || r.Changed != 1 {
		t.Fatalf("a visible changed pixel must fail: %+v %v", r, e)
	}
}
func TestDimensionsMustMatch(t *testing.T) {
	a, b := image.NewRGBA(image.Rect(0, 0, 10, 10)), image.NewRGBA(image.Rect(0, 0, 9, 10))
	if _, e := compare(a, b, 16, 0.2); e == nil {
		t.Fatal("different window geometry must be rejected")
	}
}
