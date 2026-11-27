package server

import "testing"

func TestProfileGives(t *testing.T) {
	f := func(x float64) *float64 { return &x }
	defs := []arrQualityDef{
		{Quality: arrQuality{ID: 7, Resolution: 1080}, MinSize: 3, PreferredSize: f(60), MaxSize: f(400)},
		{Quality: arrQuality{ID: 19, Resolution: 2160}, MinSize: 8, MaxSize: f(400)},
	}
	// Cutoff on a single quality: its resolution and preferred size.
	p := arrProfile{Cutoff: 7, Items: []arrProfileItem{{Quality: &arrQuality{ID: 7, Resolution: 1080}}}}
	if res, mb := profileGives(p, defs); res != "1080p" || mb != 60 {
		t.Fatalf("single quality: %q %v", res, mb)
	}
	// Cutoff on a group: the best quality in it; no preferred size means halfway between the limits.
	g := arrProfile{Cutoff: 1001, Items: []arrProfileItem{{ID: 1001, Items: []arrProfileItem{
		{Quality: &arrQuality{ID: 7, Resolution: 1080}}, {Quality: &arrQuality{ID: 19, Resolution: 2160}},
	}}}}
	if res, mb := profileGives(g, defs); res != "4K" || mb != 204 {
		t.Fatalf("group: %q %v", res, mb)
	}
	// Limits set to unlimited say nothing about size: resolution only.
	big := []arrQualityDef{{Quality: arrQuality{ID: 7, Resolution: 1080}, MinSize: 3, PreferredSize: f(1999), MaxSize: f(2000)}}
	if res, mb := profileGives(p, big); res != "1080p" || mb != 0 {
		t.Fatalf("unlimited: %q %v", res, mb)
	}
	// Unknown cutoff: nothing.
	if res, mb := profileGives(arrProfile{Cutoff: 99}, defs); res != "" || mb != 0 {
		t.Fatalf("unknown: %q %v", res, mb)
	}
}
