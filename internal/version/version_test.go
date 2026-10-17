package version

import "testing"

func TestFull(t *testing.T) {
	v, b, d := Version, Build, Dev
	t.Cleanup(func() { Version, Build, Dev = v, b, d })
	Version, Build, Dev = "0.1.0", "229", ""
	if Full() != "0.1.0.229" {
		t.Error(Full())
	}
	Dev = "1"
	if Full() != "0.1.0.229-dev" {
		t.Error(Full())
	}
}
