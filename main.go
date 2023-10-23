package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/caarlos0/env/v6"

	"github.com/go-chi/chi/v5"
	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

type StaticMD struct {
	parser   goldmark.Markdown
	content  fs.FS
	staticFS fs.FS
	templ    *template.Template
	debug    bool
}

type TemplParams struct {
	Content  template.HTML
	Metadata map[string]interface{}
}

func (s *StaticMD) LoadTemplates() (err error) {
	s.templ, err = template.ParseFS(s.content, "templ/*")
	return
}

func (s *StaticMD) GetRouter() http.Handler {
	r := chi.NewRouter()

	if err := s.LoadTemplates(); err != nil {
		fmt.Println(err)
		return nil
	}

	if s.debug {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := s.LoadTemplates(); err != nil {
					fmt.Println(err)
					return
				}
				next.ServeHTTP(w, r)
			})
		})
	}

	r.Mount("/static", http.StripPrefix("/static", http.FileServer(http.FS(s.staticFS))))
	r.Mount("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pth := path.Clean(strings.Trim(r.URL.Path, "/"))

		val, err := fs.Stat(s.content, pth)
		if err == nil && val.IsDir() {
			pth = path.Join(pth, "index")
		}

		if strings.HasSuffix(pth, ".md") { // the request wants the raw md file
			file, err := s.content.Open(pth)
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}
			defer file.Close()

			st, err := file.Stat()
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}

			http.ServeContent(w, r, st.Name(), st.ModTime(), file.(io.ReadSeeker))
			io.Copy(w, file)
			return
		}

		// check if an .md file exists, and if so, render it
		npath := pth + ".md"
		md, err := fs.ReadFile(s.content, npath)
		if err == nil {
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

			if err := s.templ.ExecuteTemplate(w, "page.templ", t); err != nil {
				fmt.Println(err)
			}
			return
		}

		// try and serve a file that has just the content
		npath = pth + ".body"
		chtm, err := fs.ReadFile(s.content, npath)
		if err == nil {
			t := TemplParams{
				Content:  template.HTML(chtm),
				Metadata: nil,
			}
			if err := s.templ.ExecuteTemplate(w, "page.templ", t); err != nil {
				fmt.Println(err)
			}
			return
		}

		// try and serve html content
		npath = pth + ".html"
		htm, err := s.content.Open(npath)
		if err == nil {
			defer htm.Close()

			st, err := htm.Stat()
			if err != nil {
				fmt.Println(err)
				return
			}

			http.ServeContent(w, r, st.Name(), st.ModTime(), htm.(io.ReadSeeker))
			return
		}

		// try and serve a regular file
		file, err := s.content.Open(pth)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			http.Error(w, "Not Found", 404)
			return
		} else if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
		st, err := file.Stat()
		if err != nil {
			fmt.Println(err)
			return
		}

		http.ServeContent(w, r, st.Name(), st.ModTime(), file.(io.ReadSeeker))
	}))

	return r
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

	staticFS, err := fs.Sub(ffs, "static")
	if err != nil {
		return nil, err
	}

	contentFS, err := fs.Sub(ffs, "content")
	if err != nil {
		return nil, err
	}

	return &StaticMD{parser: md, content: contentFS, debug: debug, staticFS: staticFS}, nil
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
