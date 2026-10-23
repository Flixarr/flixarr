package version

import "testing"

func TestFull(t *testing.T) {
	v, p, b := Version, Pre, Build
	t.Cleanup(func() { Version, Pre, Build = v, p, b })
	Version, Pre, Build = "0.2.0", "", "245"
	if Full() != "0.2.0" {
		t.Error(Full())
	}
	Pre = "dev"
	if Full() != "0.2.0-dev.245" {
		t.Error(Full())
	}
	Pre = "nightly"
	if Full() != "0.2.0-nightly.245" {
		t.Error(Full())
	}
}
