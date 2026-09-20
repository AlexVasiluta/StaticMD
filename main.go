package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/caarlos0/env/v6"

	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

type StaticMD struct {
	parser  goldmark.Markdown
	content fs.FS
	debug   bool
}

type TemplParams struct {
	Content  template.HTML
	Metadata map[string]interface{}
}

func (s *StaticMD) GetRouter() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pth := path.Clean(strings.Trim(r.URL.Path, "/"))

		if strings.HasPrefix(pth, "static") {
			http.ServeFileFS(w, r, s.content, pth)
			return
		}

		templ, err := template.ParseFS(s.content, "templ/*")
		if err != nil {
			fmt.Println(err)
			http.Error(w, "Could not compile template", http.StatusInternalServerError)
			return
		}

		val, err := fs.Stat(s.content, pth)
		if err == nil && val.IsDir() {
			pth = path.Join(pth, "index")
		}

		if strings.HasSuffix(pth, ".md") { // the request wants the raw md file
			http.ServeFileFS(w, r, s.content, pth)
			return
		}

		// check if an .md file exists, and if so, render it
		if md, err := fs.ReadFile(s.content, pth+".md"); err == nil {
			ctx := parser.NewContext()
			var buf bytes.Buffer
			if err := s.parser.Convert(md, &buf, parser.WithContext(ctx)); err != nil {
				http.Error(w, "Internal Server Error", 500)
				return
			}

			t := TemplParams{
				Content:  template.HTML(buf.String()),
				Metadata: meta.Get(ctx),
			}

			if err := templ.ExecuteTemplate(w, "page.templ", t); err != nil {
				fmt.Println(err)
			}
			return
		}

		// try and serve a file that has just the content
		if chtm, err := fs.ReadFile(s.content, pth+".body"); err == nil {
			t := TemplParams{
				Content:  template.HTML(chtm),
				Metadata: nil,
			}
			if err := templ.ExecuteTemplate(w, "page.templ", t); err != nil {
				fmt.Println(err)
			}
			return
		}

		// try and serve html content
		if _, err := fs.ReadFile(s.content, pth+".html"); err == nil {
			http.ServeFileFS(w, r, s.content, pth+".html")
			return
		}

		// try and serve a regular file
		http.ServeFileFS(w, r, s.content, pth)
	})

}

func New(debug bool, ffs fs.FS) (*StaticMD, error) {

	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			ghtml.WithHardWraps(),
			ghtml.WithUnsafe(),
		),
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			meta.Meta,
			mathjax.MathJax,
			highlighting.NewHighlighting(
				highlighting.WithStyle("xcode"),
			),
		),
	)

	return &StaticMD{parser: md, content: ffs, debug: debug}, nil
}

type config struct {
	Port  int    `env:"VRO_PORT" envDefault:"7000"`
	Debug bool   `env:"VRO_DEBUG" envDefault:"false"`
	Path  string `env:"VRO_PATH" envDefault:"/data"`
}

func main() {
	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}

	staticMD, err := New(cfg.Debug, os.DirFS(cfg.Path))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Listening on port %d\n", cfg.Port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), staticMD.GetRouter()); err != nil {
		log.Fatal(err)
	}
}
