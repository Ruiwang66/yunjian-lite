package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"encoding/json"
	"strings"
	"testing"
)

func TestServerCountryLocationPermissionsAndPrivacy(t *testing.T) {
	f := newManagementFixture(t)
	for _, tc := range []struct {
		role, code string
		status     int
	}{{"", "US", 401}, {"viewer", "US", 403}, {"operator", "ZZ", 400}, {"operator", "cn", 200}, {"admin", "HK", 200}, {"admin", "", 200}} {
		w := f.call("PATCH", "/api/servers/1/location", map[string]any{"country_code": tc.code}, tc.role, "")
		if w.Code != tc.status {
			t.Fatalf("%s %q: %d %s", tc.role, tc.code, w.Code, w.Body.String())
		}
	}
	w := f.call("PATCH", "/api/servers/1/location", map[string]any{"country_code": "DE"}, "admin", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = f.call("PATCH", "/api/servers/1", map[string]any{"name": "Renamed"}, "admin", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var s models.Server
	f.db.First(&s)
	if s.CountryCode != "DE" || s.Name != "Renamed" {
		t.Fatal("rename lost map metadata")
	}
	w = f.call("GET", "/api/public/servers/1", nil, "", "")
	var view map[string]any
	json.Unmarshal(w.Body.Bytes(), &view)
	if w.Code != 200 || view["country_code"] != "DE" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, key := range []string{"ip", "token", "system_json", "management_json"} {
		if _, found := view[key]; found {
			t.Fatal("public leak", key)
		}
	}
	w = f.call("POST", "/api/servers", map[string]any{"name": "Map server", "country_code": "sg"}, "operator", "")
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"country_code":"SG"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = f.call("POST", "/api/servers", map[string]any{"name": "No location"}, "operator", "")
	if w.Code != 201 {
		t.Fatal("legacy creation broke", w.Code)
	}
	for _, code := range []string{"CN", "US", "HK", "MO", "SG", "TW", "DE", "AU", ""} {
		if !shared.ValidCountryCode(code) {
			t.Fatal("missing map region", code)
		}
	}
	var count int64
	f.db.Model(&models.OperationLog{}).Where("action = ?", "PATCH /api/servers/:id/location").Count(&count)
	if count < 5 {
		t.Fatal("location audit missing")
	}
}
