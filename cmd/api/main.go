package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/api"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("api", flag.ContinueOnError)
	addr := flags.String("addr", ":8080", "HTTP listen address")
	dbPath := flags.String("db", "data/app.db", "SQLite database path")
	dataRoot := flags.String("data-root", "data/market", "dataset output root")
	webRoot := flags.String("web-root", "web", "static web assets directory")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	repository, err := strategy.OpenSQLiteRepository(context.Background(), *dbPath)
	if err != nil {
		return err
	}
	defer repository.Close()

	if ids, err := repository.ListIDs(context.Background()); err == nil && len(ids) == 0 {
		if raw, err := os.ReadFile("strategies/ema-boll-rsi.json"); err == nil {
			if def, err := strategy.Decode(raw); err == nil {
				_ = repository.Save(context.Background(), def, raw)
			}
		}
	}

	apiServer := api.NewServer(repository, indicator.MustBuiltinRegistry(), *dataRoot)
	apiServer.SetWebRoot(*webRoot)

	server := &http.Server{
		Addr:              *addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("crypto-backtest api listening on %s\n", *addr)
	return server.ListenAndServe()
}
