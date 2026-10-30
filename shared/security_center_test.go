package shared

import "testing"

func TestSecurityGradeUnknownAndCritical(t *testing.T) {
	o := DefaultSecurityOptions()
	if !o.Valid() {
		t.Fatal("default options")
	}
	g := GradeSecurity([]SecurityCheck{{Dimension: "system", Status: "pass"}, {Dimension: "docker", Status: "not_installed"}, {Dimension: "network", Status: "unknown"}}, o)
	if g.Score == nil || *g.Score != 100 || g.Checked != 1 || g.Unknown != 1 || g.NotInstalled != 1 {
		t.Fatalf("unknown/absent counted as passes: %+v", g)
	}
	if GradeSecurity([]SecurityCheck{{Dimension: "system", Status: "error"}}, o).Score != nil {
		t.Fatal("unmeasured score fabricated")
	}
	g = GradeSecurity([]SecurityCheck{{Dimension: "system", Status: "risk", Severity: "critical"}, {Dimension: "network", Status: "pass"}}, o)
	if g.Risk != "critical" || g.Critical != 1 || *g.Score != 70 {
		t.Fatalf("critical did not override: %+v", g)
	}
	o.Weights["system"] = 100
	if GradeSecurity([]SecurityCheck{{Dimension: "system", Status: "risk", Severity: "critical"}, {Dimension: "network", Status: "pass"}}, o).Risk != "critical" {
		t.Fatal("weight hid critical")
	}
	o.Weights["shell"] = 1
	if o.Valid() {
		t.Fatal("unrecognized dimension")
	}
}
