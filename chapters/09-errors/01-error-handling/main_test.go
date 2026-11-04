package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestDivide(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want int
		cause      error
	}{
		{"success", 10, 2, 5, nil},
		{"valid zero", 0, 2, 0, nil},
		{"negative truncation", -7, 2, -3, nil},
		{"upper boundary", 1000, 1, 1000, nil},
		{"lower boundary", -1000, -1, 1000, nil},
		{"zero divisor", 10, 0, 0, ErrDivisionByZero},
		{"too high", 1001, 1, 0, ErrDividendOutOfRange},
		{"too low", -1001, 1, 0, ErrDividendOutOfRange},
		{"divisor checked first", 1001, 0, 0, ErrDivisionByZero},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := divide(tt.a, tt.b)
			if got != tt.want || !errors.Is(err, tt.cause) {
				t.Fatalf("divide(%d, %d) = (%d, %v), want (%d, %v)", tt.a, tt.b, got, err, tt.want, tt.cause)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		text  string
		want  int
		cause error
	}{
		{"1", 1, nil},
		{"8080", 8080, nil},
		{"65535", 65535, nil},
		{"0", 0, ErrInvalidPort},
		{"-1", 0, ErrInvalidPort},
		{"65536", 0, ErrInvalidPort},
		{"", 0, strconv.ErrSyntax},
		{"not-a-number", 0, strconv.ErrSyntax},
		{"80.5", 0, strconv.ErrSyntax},
		{" 8080 ", 0, strconv.ErrSyntax},
		{strings.Repeat("9", 100), 0, strconv.ErrRange},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := parsePort(tt.text)
			if got != tt.want || !errors.Is(err, tt.cause) {
				t.Fatalf("parsePort(%q) = (%d, %v), want (%d, %v)", tt.text, got, err, tt.want, tt.cause)
			}
			if err == nil {
				return
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("parse port %q", tt.text)) {
				t.Errorf("error lacks input context: %v", err)
			}
			if tt.cause == strconv.ErrSyntax || tt.cause == strconv.ErrRange {
				var numberErr *strconv.NumError
				if !errors.As(err, &numberErr) || numberErr.Num != tt.text {
					t.Errorf("error does not preserve strconv.NumError for %q: %v", tt.text, err)
				}
			}
		})
	}
}

func TestFindUserName(t *testing.T) {
	name, err := findUserName(1)
	if name != "Ana" || err != nil {
		t.Fatalf("findUserName(1) = (%q, %v)", name, err)
	}
	name, err = findUserName(99)
	if name != "" || !errors.Is(err, ErrNotFound) {
		t.Fatalf("findUserName(99) = (%q, %v), want empty name and ErrNotFound", name, err)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("lookup error lacks ID context: %v", err)
	}
}

func TestValidateSignup(t *testing.T) {
	tests := []struct {
		name        string
		age         int
		invalidName bool
		invalidAge  bool
	}{
		{"Ana", 18, false, false},
		{" Ana ", 25, false, false},
		{"", 18, true, false},
		{" \t\u2003", 18, true, false},
		{"Ana", 17, false, true},
		{"", -1, true, true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q age %d", tt.name, tt.age), func(t *testing.T) {
			err := validateSignup(tt.name, tt.age)
			if !tt.invalidName && !tt.invalidAge {
				if err != nil {
					t.Fatalf("valid signup returned %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("validation error = %v, want ErrInvalidInput", err)
			}
			if strings.Contains(err.Error(), "name must not be blank") != tt.invalidName ||
				strings.Contains(err.Error(), "age must be at least 18") != tt.invalidAge {
				t.Errorf("validation error does not describe all and only the invalid fields: %v", err)
			}
		})
	}
}

func TestChargePayment(t *testing.T) {
	for _, amount := range []int{-1, 0, 1, 2500} {
		t.Run(strconv.Itoa(amount), func(t *testing.T) {
			err := chargePayment(amount)
			if amount > 0 {
				if err != nil {
					t.Fatalf("successful charge returned a non-nil error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("nonpositive charge returned nil")
			}
			var detail *OperationError
			if !errors.As(fmt.Errorf("checkout: %w", err), &detail) {
				t.Fatalf("error %v does not expose *OperationError", err)
			}
			if detail.Op != "charge payment" || detail.Code != 400 || detail.Message == "" {
				t.Errorf("missing payment error details: %+v", detail)
			}
		})
	}
}

func TestProcessAndClose(t *testing.T) {
	processErr := errors.New("work failed")
	closeErr := errors.New("close failed")
	tests := []struct {
		name       string
		processErr error
		closeErr   error
	}{
		{"success", nil, nil},
		{"work fails", processErr, nil},
		{"cleanup fails", nil, closeErr},
		{"both fail", processErr, closeErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &fakeFile{Name: "report.txt", processErr: tt.processErr, closeErr: tt.closeErr}
			err := processAndClose(file)
			if !file.closed {
				t.Error("file was not closed")
			}
			if tt.processErr == nil && tt.closeErr == nil {
				if err != nil {
					t.Fatalf("successful work and cleanup returned %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("failure was ignored")
			}
			if errors.Is(err, processErr) != (tt.processErr != nil) ||
				errors.Is(err, closeErr) != (tt.closeErr != nil) {
				t.Errorf("returned error does not preserve all and only the failures: %v", err)
			}
			if !strings.Contains(err.Error(), file.Name) {
				t.Errorf("error lacks file context: %v", err)
			}
		})
	}
}

func TestProcessFakeFile(t *testing.T) {
	if err := processFakeFile("config.json"); err != nil {
		t.Fatalf("valid file returned %v", err)
	}
	for _, name := range []string{"", " \t"} {
		if err := processFakeFile(name); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("processFakeFile(%q) = %v, want ErrInvalidInput", name, err)
		}
	}
}
