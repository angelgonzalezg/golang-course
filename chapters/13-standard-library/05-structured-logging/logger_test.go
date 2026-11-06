package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func decodeRecords(t *testing.T, data string) []map[string]any {
	t.Helper()
	decoder := jsontext.NewDecoder(strings.NewReader(data))
	var records []map[string]any
	for {
		var record map[string]any
		err := json.UnmarshalDecode(decoder, &record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatalf("invalid JSON log record: %v", err)
		}
		records = append(records, record)
	}
}

func TestLoggerConfigurationAndRepresentationPolicies(t *testing.T) {
	before := slog.Default()
	for _, format := range []string{"", "text", "json"} {
		t.Run("format="+format, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := newLogger(&output, loggerConfig{Format: format})
			if err != nil {
				t.Fatal(err)
			}
			logger.Info("configured",
				slog.Any("credential", Secret("private credential")),
				slog.Group("auth", slog.String("token", "private token")),
				slog.Group("game", slog.Int("level", 7)))
			if strings.Contains(output.String(), "private credential") || strings.Contains(output.String(), "private token") {
				t.Fatal("configured value/key policy exposed the sample secret")
			}
			if format == "json" {
				records := decodeRecords(t, output.String())
				if len(records) != 1 {
					t.Fatalf("got %d records, want one", len(records))
				}
				record := records[0]
				if record["severity"] != "INFO" || record["level"] != nil || record["credential"] != "[REDACTED]" {
					t.Fatalf("unexpected root attributes: %v", record)
				}
				auth, ok := record["auth"].(map[string]any)
				if !ok || auth["token"] != "[REDACTED]" {
					t.Fatalf("nested token policy missing: %v", record)
				}
				game, ok := record["game"].(map[string]any)
				if !ok || game["level"] != float64(7) || game["severity"] != nil {
					t.Fatalf("root renaming changed domain attributes: %v", record)
				}
			} else {
				for _, part := range []string{"severity=INFO", "auth.token=", "game.level=7"} {
					if !strings.Contains(output.String(), part) {
						t.Fatalf("missing %q in %s", part, output.String())
					}
				}
			}
		})
	}
	if logger, err := newLogger(io.Discard, loggerConfig{Format: "xml"}); logger != nil || !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("unknown format returned (%v, %v)", logger, err)
	}
	if slog.Default() != before {
		t.Fatal("constructor changed the global default logger")
	}
}

func TestDerivedLoggersKeepMetadataAndDynamicThreshold(t *testing.T) {
	var output bytes.Buffer
	var minimum slog.LevelVar
	logger, err := newLogger(&output, loggerConfig{Format: "json", Level: &minimum})
	if err != nil {
		t.Fatal(err)
	}
	service := logger.With("service", "checkout")
	request := service.WithGroup("request").With("id", "req-1")
	request.Debug("filtered")
	request.Info("completed", slog.Int("status", 200))
	service.Info("parent unchanged")
	minimum.Set(slog.LevelDebug)
	request.Debug("enabled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.With("request_id", "req-2").InfoContext(ctx, "canceled work observed")
	records := decodeRecords(t, output.String())
	if len(records) != 4 {
		t.Fatalf("got %d records, want four with initial DEBUG filtered", len(records))
	}
	for _, record := range records {
		if record["service"] != "checkout" {
			t.Fatalf("shared metadata lost: %v", record)
		}
	}
	group, ok := records[0]["request"].(map[string]any)
	if !ok || group["id"] != "req-1" || group["status"] != float64(200) {
		t.Fatalf("request metadata lost: %v", records[0])
	}
	if records[1]["request"] != nil || records[2]["severity"] != "DEBUG" || records[3]["request_id"] != "req-2" {
		t.Fatalf("unexpected scope, level, or cancellation behavior: %v", records)
	}
}

func TestLazyValueIsResolvedOnlyForEnabledRecords(t *testing.T) {
	var output bytes.Buffer
	var minimum slog.LevelVar
	logger, err := newLogger(&output, loggerConfig{Format: "json", Level: &minimum})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	value := lazyValue(func() slog.Value {
		calls++
		return slog.StringValue("computed")
	})
	logger.Debug("filtered", slog.Any("snapshot", value))
	if calls != 0 || output.Len() != 0 {
		t.Fatal("disabled record evaluated or emitted the deferred value")
	}
	minimum.Set(slog.LevelDebug)
	logger.Debug("enabled", slog.Any("snapshot", value))
	records := decodeRecords(t, output.String())
	if calls != 1 || len(records) != 1 || records[0]["snapshot"] != "computed" {
		t.Fatalf("calls=%d records=%v", calls, records)
	}
}

func TestProcessOrderLogsOneOutcomeAndReturnsTheCause(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(&output, loggerConfig{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	logger = logger.With("request_id", "req-5")
	if err := validateOrder("order-1", 0); !errors.Is(err, ErrInvalidOrder) || output.Len() != 0 {
		t.Fatal("lower-level validation must return its cause without logging")
	}
	if err := processOrder(context.Background(), logger, "order-1", 2); err != nil {
		t.Fatal(err)
	}
	if err := processOrder(context.Background(), logger, "order-2", 0); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("operation lost validation cause: %v", err)
	}
	records := decodeRecords(t, output.String())
	if len(records) != 2 || records[0]["severity"] != "INFO" || records[1]["severity"] != "ERROR" {
		t.Fatalf("expected one outcome per call: %v", records)
	}
	if records[0]["items"] != float64(2) || records[1]["order_id"] != "order-2" || records[1]["request_id"] != "req-5" {
		t.Fatalf("missing operation metadata: %v", records)
	}
	if message, ok := records[1]["error"].(string); !ok || !strings.Contains(message, ErrInvalidOrder.Error()) {
		t.Fatalf("missing error description: %v", records[1])
	}
}

func TestSourceLocationIsOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var output bytes.Buffer
		logger, err := newLogger(&output, loggerConfig{Format: "json", AddSource: enabled})
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("located")
		records := decodeRecords(t, output.String())
		source, ok := records[0]["source"].(map[string]any)
		if !enabled {
			if ok {
				t.Fatal("source emitted without being enabled")
			}
			continue
		}
		if !ok {
			t.Fatal("source metadata missing")
		}
		file, fileOK := source["file"].(string)
		line, lineOK := source["line"].(float64)
		if !fileOK || filepath.Base(file) != "logger_test.go" || !lineOK || line <= 0 {
			t.Fatalf("source did not identify the logging call: %v", source)
		}
	}
}
