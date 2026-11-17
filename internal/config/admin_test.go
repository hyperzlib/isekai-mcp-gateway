package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAdminSubsConfiguration(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{`{}`, []string{}},
		{`{"admin":{"subs":[]}}`, []string{}},
		{`{"admin":{"subs":["one","two"]}}`, []string{"one", "two"}},
	} {
		c := Defaults()
		if err := json.Unmarshal([]byte(tc.input), &c.Static); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.Static.Admin.Subs, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.input, c.Static.Admin.Subs, tc.want)
		}
	}
}
