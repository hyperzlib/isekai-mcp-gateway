package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSiteConfiguration(t *testing.T) {
	c := Defaults()
	if err := json.Unmarshal([]byte(`{"configVersion":1}`), &c.App); err != nil {
		t.Fatal(err)
	}
	if err := c.validateSite(); err != nil || c.App.Site.Title != DefaultSiteTitle || c.App.Site.IconURL != "" {
		t.Fatal("old config defaults", err)
	}
	for _, title := range []string{"", " \t\n", strings.Repeat("站", 256), "<b>Site</b>"} {
		c.App.Site.Title = title
		if err := c.validateSite(); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(title) == "" && c.App.Site.Title != DefaultSiteTitle {
			t.Fatal("blank title fallback")
		}
	}
	c.App.Site.Title = strings.Repeat("站", 257)
	if c.validateSite() == nil {
		t.Fatal("overlong title accepted")
	}
	for _, raw := range []string{"", "/assets/icon.svg", "/assets/icon.svg?v=2", "https://images.example:8443/icon.svg?v=2"} {
		if !ValidSiteIcon(raw) {
			t.Fatal("valid icon rejected", raw)
		}
	}
	for _, raw := range []string{"//evil.example/icon", "assets/icon.svg", "http://images.example/icon", "https://user:password@images.example/icon", "/icon.svg#x", "/\\evil.example/icon", "https://images.example/icon; img-src *", "https://images.example/icon\n", "data:image/png;base64,x", "/icon\u2028.svg"} {
		if ValidSiteIcon(raw) {
			t.Fatal("unsafe icon accepted", raw)
		}
		c.App.Site = Site{Title: DefaultSiteTitle, IconURL: raw}
		if c.validateSite() == nil {
			t.Fatal("unsafe config accepted", raw)
		}
	}
}

func TestSiteHeaderLink(t *testing.T) {
	for _, tc := range []struct {
		link  *SiteHeaderLink
		valid bool
	}{
		{nil, true}, {&SiteHeaderLink{Label: "百科", URL: "https://www.isekai.cn"}, true},
		{&SiteHeaderLink{Label: "Wiki", URL: "/wiki?x=1#top"}, true},
		{&SiteHeaderLink{Label: " ", URL: "/"}, false}, {&SiteHeaderLink{Label: "Wiki"}, false},
		{&SiteHeaderLink{Label: "Wiki", URL: "//evil.test"}, false}, {&SiteHeaderLink{Label: "Wiki", URL: "javascript:alert(1)"}, false},
		{&SiteHeaderLink{Label: "Wiki", URL: "https://user@evil.test"}, false}, {&SiteHeaderLink{Label: "Wiki", URL: "http://evil.test"}, false},
	} {
		c := Defaults()
		c.App.Site.HeaderLink = tc.link
		if (c.validateSite() == nil) != tc.valid {
			t.Errorf("link %#v valid=%v", tc.link, tc.valid)
		}
	}
}
