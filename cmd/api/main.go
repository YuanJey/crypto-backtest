package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/bytedance/crypto-backtest/internal/api"
	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/strategy"
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

	server := &http.Server{
		Addr:              *addr,
		Handler:           api.NewServer(repository, indicator.MustBuiltinRegistry(), *dataRoot).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("crypto-backtest api listening on %s\n", *addr)
	return server.ListenAndServe()
}
