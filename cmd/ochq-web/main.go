// Command ochq-web serves the ochq.uz landing page and its comments, and
// moderates the comments from the command line:
//
//	ochq-web [serve]        run the web server
//	ochq-web list [-n 20]   show the newest comments, hidden ones included
//	ochq-web hide <id>      hide a comment (the running server sees it at once)
//	ochq-web unhide <id>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mukhiddinov/ochq/internal/web"
	"github.com/mukhiddinov/ochq/pkg/version"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("ochq-web "+cmd, flag.ExitOnError)
	db := fs.String("db", envOr("OCHQ_WEB_DB", "/var/lib/ochq-web/comments.jsonl"), "comment log file [$OCHQ_WEB_DB]")
	var err error
	switch cmd {
	case "serve":
		addr := fs.String("addr", envOr("OCHQ_WEB_ADDR", "127.0.0.1:8090"), "listen address [$OCHQ_WEB_ADDR]")
		proxy := fs.String("trusted-proxy", envOr("OCHQ_WEB_TRUSTED_PROXY", "172.30.0.2"), "only this peer may set X-Forwarded-For [$OCHQ_WEB_TRUSTED_PROXY]")
		showVersion := fs.Bool("version", false, "print version and exit")
		fs.Parse(args)
		if *showVersion {
			fmt.Println("ochq-web", version.Version)
			return
		}
		err = serve(*addr, *db, *proxy, os.Getenv("OCHQ_WEB_SALT"))
	case "list":
		n := fs.Int("n", 20, "how many comments")
		fs.Parse(args)
		err = listComments(*db, *n)
	case "hide", "unhide":
		// Accept the id before or after the flags: hide 3 -db x, hide -db x 3.
		id := ""
		if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
			id, args = args[0], args[1:]
		}
		fs.Parse(args)
		if id == "" {
			id = fs.Arg(0)
		}
		err = setHidden(*db, id, cmd == "hide")
	default:
		fmt.Fprintf(os.Stderr, "ochq-web: unknown command %q (serve, list, hide, unhide)\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ochq-web:", err)
		os.Exit(1)
	}
}

func serve(addr, db, proxy, salt string) error {
	if salt == "" {
		fmt.Fprintln(os.Stderr, "ochq-web: OCHQ_WEB_SALT is not set (generate one: openssl rand -hex 32)")
		os.Exit(2)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv, err := web.NewServer(web.Config{DBPath: db, Salt: salt, TrustedProxy: proxy})
	if err != nil {
		return err
	}
	defer srv.Close()

	hs := &http.Server{Addr: addr, Handler: srv, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	logger.Info("ochq-web listening", "addr", addr, "version", version.Version)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func listComments(db string, n int) error {
	s, err := web.OpenStore(db)
	if err != nil {
		return err
	}
	defer s.Close()
	cs, err := s.List(0, n, true)
	if err != nil {
		return err
	}
	for _, c := range cs {
		name, state := c.Name, ""
		if name == "" {
			name = "ghost"
		}
		if c.Hidden {
			state = " [hidden]"
		}
		fmt.Printf("%d  %s  %s%s  %s: %q\n", c.ID, c.CreatedAt.Format(time.DateTime), c.Lang, state, name, c.Body)
	}
	return nil
}

func setHidden(db, arg string, hidden bool) error {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return fmt.Errorf("usage: ochq-web hide|unhide <id>")
	}
	s, err := web.OpenStore(db)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.SetHidden(id, hidden)
}
