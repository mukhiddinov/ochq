package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"strings"

	"github.com/mukhiddinov/ochq/internal/ui"
	"github.com/mukhiddinov/ochq/pkg/version"
	site "github.com/mukhiddinov/ochq/web"
)

const origin = "https://ochq.uz"

// lang is one translation of the page.
type lang struct {
	Lang, Path, Label, Locale string
}

var langs = []lang{
	{"uz", "/", "UZ", "uz_UZ"},
	{"en", "/en/", "EN", "en_US"},
	{"ru", "/ru/", "RU", "ru_RU"},
}

// jsKeys are the strings site.js needs; they go into the page as JSON.
var jsKeys = []string{"copy", "copied", "cap1", "cap2", "cap3", "cap4", "ghost",
	"err_rate", "err_bad", "err_net", "f_send", "f_sending", "empty"}

type alternate struct {
	Lang, Path, URL, Label string
}

type pageData struct {
	Lang, Path, Canonical, Locale, Version string
	T                                      map[string]template.HTML
	JS                                     map[string]string
	LD                                     map[string]any
	Alternates                             []alternate
}

// releaseVersion is shown in the install demo; development builds show none.
var releaseVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// renderPages renders every translation once, at startup, so a broken
// template or a missing string stops the server instead of a visitor.
func renderPages() (map[string][]byte, error) {
	tmpl, err := template.New("page.html.tmpl").Option("missingkey=error").ParseFS(site.FS, "site/page.html.tmpl")
	if err != nil {
		return nil, err
	}
	var alts []alternate
	for _, l := range langs {
		alts = append(alts, alternate{l.Lang, l.Path, origin + l.Path, l.Label})
	}
	ver := version.Version
	if !releaseVersion.MatchString(ver) {
		ver = ""
	}
	pages := map[string][]byte{}
	for _, l := range langs {
		strs, err := loadStrings(l.Lang)
		if err != nil {
			return nil, err
		}
		d := pageData{
			Lang: l.Lang, Path: l.Path, Canonical: origin + l.Path, Locale: l.Locale, Version: ver,
			T: map[string]template.HTML{}, JS: map[string]string{}, Alternates: alts,
		}
		for k, v := range strs {
			d.T[k] = template.HTML(v) // our own translations; they may hold markup
		}
		for _, k := range jsKeys {
			v, ok := strs[k]
			if !ok {
				return nil, fmt.Errorf("web: %s.json lacks %q", l.Lang, k)
			}
			d.JS[k] = v
		}
		d.LD = map[string]any{
			"@context": "https://schema.org", "@type": "SoftwareApplication",
			"name": "ochq", "url": d.Canonical, "description": strs["description"],
			"applicationCategory": "DeveloperApplication", "operatingSystem": "Linux, macOS, Windows",
			"offers":     map[string]string{"@type": "Offer", "price": "0", "priceCurrency": "USD"},
			"author":     map[string]string{"@type": "Person", "name": "Khusniddin Mukhiddinov", "url": "https://github.com/mukhiddinov"},
			"license":    "https://opensource.org/licenses/MIT",
			"inLanguage": l.Lang,
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, d); err != nil {
			return nil, fmt.Errorf("web: render %s: %w", l.Lang, err)
		}
		pages[l.Path] = buf.Bytes()
	}
	return pages, nil
}

func loadStrings(lang string) (map[string]string, error) {
	b, err := fs.ReadFile(site.FS, "i18n/"+lang+".json")
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("web: %s.json: %w", lang, err)
	}
	return m, nil
}

// bannerJSON is web/site/assets/banner.json (see TestBannerJSONInSync).
type bannerJSON struct {
	Frames  []ui.Frame `json:"frames"`
	Final   [6]string  `json:"final"`
	Colors  [6]string  `json:"colors"`
	Tagline string     `json:"tagline"`
}

func bannerData() bannerJSON {
	return bannerJSON{Frames: ui.AnimationFrames(), Final: ui.FinalFrame(), Colors: ui.RowHex, Tagline: ui.Tagline}
}

func (s *Server) routePages() error {
	pages, err := renderPages()
	if err != nil {
		return err
	}
	for path, body := range pages {
		s.mux.HandleFunc("GET "+path+"{$}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.Write(body)
		})
		if path != "/" { // /en -> /en/, permanently, so search engines keep one URL
			s.mux.Handle("GET "+strings.TrimSuffix(path, "/"), http.RedirectHandler(path, http.StatusMovedPermanently))
		}
	}
	static, err := fs.Sub(site.FS, "site")
	if err != nil {
		return err
	}
	files := http.FileServerFS(static)
	cached := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") { // no directory listings
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
	s.mux.Handle("GET /assets/", cached)
	s.mux.Handle("GET /favicon.svg", cached)
	s.mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /api/\n\nSitemap: %s/sitemap.xml\n", origin)
	})
	sitemap := sitemapXML()
	s.mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write(sitemap)
	})
	s.mux.HandleFunc("GET /install.sh", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://mukhiddinov.github.io/ochq/install.sh", http.StatusFound)
	})
	return nil
}

// sitemapXML lists every translation with its alternates, as Google
// recommends for multilingual pages.
func sitemapXML() []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, l := range langs {
		fmt.Fprintf(&b, "  <url>\n    <loc>%s%s</loc>\n", origin, l.Path)
		for _, a := range langs {
			fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=%q href=\"%s%s\"/>\n", a.Lang, origin, a.Path)
		}
		fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s/\"/>\n  </url>\n", origin)
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}
