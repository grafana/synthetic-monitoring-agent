package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/synthetic-monitoring-agent/cmd/test-api/internal/db"
	"github.com/grafana/synthetic-monitoring-agent/cmd/test-api/internal/grpc"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(filepath.Base(args[0]), flag.ExitOnError)

	var (
		debug          = flags.Bool("debug", false, "debug output (enables verbose)")
		verbose        = flags.Bool("verbose", false, "verbose logging")
		grpcListenAddr = flags.String("grpc-listen-address", ":4031", "GRPC listen address")
		dataFn         = flags.String("load-data", "data.json", "filename with data to load")
	)

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	zl := getLogger(filepath.Base(args[0]), *verbose, *debug, stdout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)

	db := db.New()

	if err := loadData(*dataFn, db); err != nil {
		return err
	}

	checksServer, err := grpc.NewChecksServer(grpc.ChecksServerOpts{
		Logger: zl.With().Str("subsystem", "checks_server").Logger(),
		Db:     db,
	})
	if err != nil {
		zl.Error().Err(err).Msg("cannot create checks server")
		return err
	}

	tenantsServer, err := grpc.NewTenantsServer(grpc.TenantsServerOpts{
		Logger: zl.With().Str("subsystem", "tenants_server").Logger(),
		Db:     db,
	})
	if err != nil {
		zl.Error().Err(err).Msg("cannot create tenants server")
		return err
	}

	grpcServer, err := grpc.NewServer(ctx, &grpc.Opts{
		Logger:        zl.With().Str("subsystem", "grpc").Logger(),
		ListenAddr:    *grpcListenAddr,
		ChecksServer:  checksServer,
		TenantsServer: tenantsServer,
		Db:            db,
	})
	if err != nil {
		zl.Error().Err(err).Msg("cannot create GRPC server")
		return err
	}

	eg.Go(func() error { return grpcServer.Run(ctx) })

	if err := eg.Wait(); err != nil {
		zl.Error().Err(err).Send()
		return err
	}

	return nil
}

func getLogger(name string, verbose, debug bool, w io.Writer) zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	zl := zerolog.New(w).With().Timestamp().Str("program", name).Logger()

	switch {
	case debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		zl = zl.With().Caller().Logger()

	case verbose:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

	default:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}

	return zl
}

func loadData(fn string, db *db.Db) error {
	var data struct {
		Tenants     []synthetic_monitoring.Tenant `json:"tenants"`
		Probes      []synthetic_monitoring.Probe  `json:"probes"`
		ProbeTokens map[int64]string              `json:"probeTokens"`
		Checks      []synthetic_monitoring.Check  `json:"checks"`
	}

	fh, err := os.Open(fn) //#nosec -- yes, I want to read whatever file the user tells me.
	if err != nil {
		return err
	}

	defer fh.Close() //#nosec -- file is open for reading.

	dec := json.NewDecoder(fh)

	if err := dec.Decode(&data); err != nil {
		return err
	}

	ctx := context.Background()

	for _, tenant := range data.Tenants {
		if err := db.AddTenant(ctx, &tenant); err != nil {
			return err
		}
	}

	for _, probe := range data.Probes {
		token, found := data.ProbeTokens[probe.Id]
		if !found {
			return fmt.Errorf("token not found for probe %d", probe.Id)
		}
		if err := db.AddProbe(ctx, &probe, []byte(token)); err != nil {
			return err
		}
	}

	for _, check := range data.Checks {
		if err := db.AddCheck(ctx, &check); err != nil {
			return err
		}
	}

	return nil
}
