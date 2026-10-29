// visualdiff enforces measured screenshot comparison. It is deliberately
// separate from MyGo source checks: source checks cannot prove visual parity.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

type Result struct {
	Base                   string  `json:"base"`
	Candidate              string  `json:"candidate"`
	Width                  int     `json:"width"`
	Height                 int     `json:"height"`
	Pixels                 int     `json:"pixels"`
	Changed                int     `json:"changed"`
	ChangedFraction        float64 `json:"changedFraction"`
	MeanChannelDifference  float64 `json:"meanChannelDifference"`
	AllowedChangedFraction float64 `json:"allowedChangedFraction"`
	Passed                 bool    `json:"passed"`
}

func load(path string) (image.Image, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	im, _, e := image.Decode(f)
	return im, e
}
func compare(a, b image.Image, tolerance uint32, allowed float64) (Result, error) {
	aa, bb := a.Bounds(), b.Bounds()
	if aa.Dx() != bb.Dx() || aa.Dy() != bb.Dy() {
		return Result{}, fmt.Errorf("image dimension mismatch: %dx%d != %dx%d", aa.Dx(), aa.Dy(), bb.Dx(), bb.Dy())
	}
	pixels := aa.Dx() * aa.Dy()
	if pixels == 0 {
		return Result{}, fmt.Errorf("empty screenshot")
	}
	var change int
	var total uint64
	for y := 0; y < aa.Dy(); y++ {
		for x := 0; x < aa.Dx(); x++ {
			r1, g1, b1, _ := a.At(aa.Min.X+x, aa.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			channels := []uint32{absDiff(r1, r2), absDiff(g1, g2), absDiff(b1, b2)}
			bad := false
			for _, v := range channels {
				total += uint64(v >> 8)
				if v>>8 > tolerance {
					bad = true
				}
			}
			if bad {
				change++
			}
		}
	}
	fraction := float64(change) / float64(pixels)
	return Result{Width: aa.Dx(), Height: aa.Dy(), Pixels: pixels, Changed: change, ChangedFraction: fraction, MeanChannelDifference: float64(total) / float64(3*pixels), AllowedChangedFraction: allowed, Passed: fraction <= allowed}, nil
}
func absDiff(a, b uint32) uint32 {
	if a >= b {
		return a - b
	}
	return b - a
}
func main() {
	ref := flag.String("reference", "", "immutable original screenshot PNG")
	current := flag.String("candidate", "", "actual MyGo native screenshot PNG")
	tolerance := flag.Uint("channel-tolerance", 0, "per-channel 0-255 difference considered changed")
	allowed := flag.Float64("max-changed", 0, "maximum fraction of changed pixels")
	flag.Parse()
	if *ref == "" || *current == "" || *allowed < 0 || *allowed > 1 || *tolerance > 255 {
		fmt.Fprintln(os.Stderr, "invalid arguments: provide reference and candidate screenshots")
		os.Exit(2)
	}
	a, e := load(*ref)
	if e != nil {
		fmt.Fprintln(os.Stderr, "reference unavailable:", e)
		os.Exit(2)
	}
	b, e := load(*current)
	if e != nil {
		fmt.Fprintln(os.Stderr, "native screenshot unavailable:", e)
		os.Exit(2)
	}
	r, e := compare(a, b, uint32(*tolerance), *allowed)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	r.Base, r.Candidate = *ref, *current
	_ = json.NewEncoder(os.Stdout).Encode(r)
	if !r.Passed {
		os.Exit(1)
	}
}
