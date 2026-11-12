// Add PostgreSQL persistence to the existing TLS gRPC service. The default
// demonstration is finite; flags support separate server/client processes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/postgresapp"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/sql-lab/contactpg"
)

// run keeps configuration and cleanup separate from os.Exit. Client/certificate
// modes need no database; only demo/server/migration modes read DATABASE_URL.
func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("grpc-postgres", flag.ContinueOnError)
	serve := flags.Bool("serve", false, "keep serving with PostgreSQL")
	addr := flags.String("addr", "127.0.0.1:50053", "listen address with -serve")
	target := flags.String("target", "", "run a finite keyed-create client against this server")
	initTLS := flags.String("init-tls", "", "write local TLS material into a new directory and exit")
	tlsDir := flags.String("tls-dir", "", "directory containing local TLS files for standalone modes")
	reflection := flags.Bool("reflection", false, "enable authenticated reflection with -serve")
	migrateOnly := flags.Bool("migrate-only", false, "apply service migrations and exit")
	key := flags.String("key", "create-1", "stable key for the intended client creation")
	name := flags.String("name", "Persistent contact", "original client name field")
	email := flags.String("email", "", "original client email field")
	phone := flags.String("phone", "", "original client phone field")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	modes := 0
	for _, chosen := range []bool{*serve, *target != "", *initTLS != "", *migrateOnly} {
		if chosen {
			modes++
		}
	}
	if flags.NArg() != 0 || modes > 1 {
		return errors.New("choose the demo, -init-tls, -migrate-only, -serve, or -target")
	}
	if *initTLS != "" {
		material, err := labtls.Generate()
		if err != nil {
			return err
		}
		if err := material.WriteDirectory(*initTLS); err != nil {
			return err
		}
		fmt.Printf("Local TLS files created in %s; certificates expire in 24 hours.\n", *initTLS)
		return nil
	}
	if *serve || *target != "" {
		if *tlsDir == "" {
			return errors.New("-tls-dir is required for standalone server/client modes")
		}
		if *target != "" {
			if os.Getenv("GRPC_LAB_WRITER_TOKEN") == "" {
				return errors.New("set GRPC_LAB_WRITER_TOKEN for the client")
			}
			transport, err := labtls.LoadClient(*tlsDir)
			if err != nil {
				return err
			}
			return postgresapp.CreateClient(ctx, *target, transport, os.Getenv("GRPC_LAB_WRITER_TOKEN"), *key,
				&contactsv1.ContactInput{Name: *name, Email: *email, Phone: *phone}, os.Stdout)
		}
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL; see extensions/01-protobuf-and-grpc/06-postgres/README.md")
	}
	if *migrateOnly {
		version, err := contactpg.Migrate(dsn)
		if err == nil {
			fmt.Printf("Service schema version: %d\n", version)
		}
		return err
	}
	if *serve {
		policy, err := rpcinfra.NewPolicy(os.Getenv("GRPC_LAB_READER_TOKEN"), os.Getenv("GRPC_LAB_WRITER_TOKEN"))
		if err != nil {
			return err
		}
		transport, err := labtls.LoadServer(*tlsDir)
		if err != nil {
			return err
		}
		return postgresapp.Serve(ctx, dsn, *addr, transport, policy, *reflection, os.Stdout)
	}
	return postgresapp.RunDemo(ctx, dsn, os.Stdout)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gRPC PostgreSQL:", err)
		os.Exit(1)
	}
}
