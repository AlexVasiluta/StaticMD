package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

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
			slog.ErrorContext(r.Context(), "Could not build compile template", slog.Any("error", err))
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
				slog.ErrorContext(r.Context(), "Could not execute template", slog.Any("error", err))
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
				slog.ErrorContext(r.Context(), "Could not execute template", slog.Any("error", err))
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

func New(ffs fs.FS) *StaticMD {

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

	return &StaticMD{parser: md, content: ffs}
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})))

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "Error running server", slog.Any("error", err))
	}
}

func run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	address := os.Getenv("VRO_ADDRESS")
	if address == "" {
		port, err := strconv.Atoi(cmp.Or(os.Getenv("VRO_PORT"), "7000"))
		if err != nil {
			return err
		}
		address = net.JoinHostPort("", strconv.Itoa(port))
	}

	staticMD := New(os.DirFS(cmp.Or(os.Getenv("VRO_PATH"), "/data")))

	server := &http.Server{
		Addr:    address,
		Handler: staticMD.GetRouter(),

		ReadHeaderTimeout: 1 * time.Minute,
	}

	slog.InfoContext(ctx, "Starting server", slog.Any("address", address))
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "Error initializing web server", slog.Any("error", err))
			cancel()
		}
	}()

	defer func() {
		slog.InfoContext(ctx, "Shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(ctx, "Error shutting down", slog.Any("error", err))
		}
	}()

	<-ctx.Done()

	return nil
}
