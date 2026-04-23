// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"go.etcd.io/bbolt"
)

const usage = `storageinspect - dump OpenTelemetry filestorage BoltDB contents

Usage:
  storageinspect [flags] <db-path>

Flags:
  -bucket string
        Bucket name to inspect (default "default")
  -format string
        Value format: text, hex, base64, raw (default "text")
  -keys-only
        Print only keys
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("storageinspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var bucketName string
	var format string
	var keysOnly bool

	fs.StringVar(&bucketName, "bucket", "default", "Bucket name to inspect")
	fs.StringVar(&format, "format", "text", "Value format: text, hex, base64, raw")
	fs.BoolVar(&keysOnly, "keys-only", false, "Print only keys")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("expected exactly one database path")
	}

	dbPath := fs.Arg(0)
	mode, err := parseFormat(format)
	if err != nil {
		return err
	}

	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{
		ReadOnly: true,
		Timeout:  time.Second,
	})
	if err != nil {
		return fmt.Errorf("open bolt db: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	return db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %q not found", bucketName)
		}

		stats := bucket.Stats()
		fmt.Printf("path: %s\n", dbPath)
		fmt.Printf("bucket: %s\n", bucketName)
		fmt.Printf("keys: %d\n", stats.KeyN)
		fmt.Println()

		return bucket.ForEach(func(k, v []byte) error {
			key := string(k)
			if v == nil {
				fmt.Printf("key_bytes: %d\n", len(k))
				fmt.Println("key:")
				fmt.Println(indent(key, "  "))
				fmt.Println("value: <nested bucket>")
				fmt.Println()
				return nil
			}

			if keysOnly {
				fmt.Println(key)
				return nil
			}

			fmt.Printf("key_bytes: %d\n", len(k))
			fmt.Println("key:")
			fmt.Println(indent(key, "  "))
			fmt.Printf("value_bytes: %d\n", len(v))
			fmt.Println("value:")
			fmt.Println(indent(formatValue(v, mode), "  "))
			fmt.Println()
			return nil
		})
	})
}

type outputFormat int

const (
	formatText outputFormat = iota
	formatHex
	formatBase64
	formatRaw
)

func parseFormat(s string) (outputFormat, error) {
	switch strings.ToLower(s) {
	case "text":
		return formatText, nil
	case "hex":
		return formatHex, nil
	case "base64":
		return formatBase64, nil
	case "raw":
		return formatRaw, nil
	default:
		return 0, fmt.Errorf("unsupported format %q", s)
	}
}

func formatValue(value []byte, mode outputFormat) string {
	switch mode {
	case formatText:
		if utf8.Valid(value) {
			return string(value)
		}
		return "value not UTF-8"
	case formatHex:
		return fmt.Sprintf("%x", value)
	case formatBase64:
		return base64.StdEncoding.EncodeToString(value)
	case formatRaw:
		return string(value)
	default:
		return fmt.Sprintf("%x", value)
	}
}

func indent(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	for idx, line := range lines {
		lines[idx] = prefix + line
	}
	return strings.Join(lines, "\n")
}
