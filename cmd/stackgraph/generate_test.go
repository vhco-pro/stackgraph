package main

import (
	"os"
	"testing"
)

func TestDetectContentType_State(t *testing.T) {
	data, err := os.ReadFile("../../testdata/state/aws/simple-vpc.json")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	result := detectContentType(data)
	if result != "state" {
		t.Errorf("expected 'state', got %q", result)
	}
}

func TestDetectContentType_DOT(t *testing.T) {
	data, err := os.ReadFile("../../testdata/dot/simple.dot")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	result := detectContentType(data)
	if result != "dot" {
		t.Errorf("expected 'dot', got %q", result)
	}
}

func TestDetectInputType_ByExtension(t *testing.T) {
	result, err := detectInputType("../../testdata/dot/simple.dot")
	if err != nil {
		t.Fatalf("detectInputType failed: %v", err)
	}
	if result != "dot" {
		t.Errorf("expected 'dot' for .dot extension, got %q", result)
	}
}

func TestDetectInputType_JSONSniffing(t *testing.T) {
	result, err := detectInputType("../../testdata/state/aws/simple-vpc.json")
	if err != nil {
		t.Fatalf("detectInputType failed: %v", err)
	}
	if result != "state" {
		t.Errorf("expected 'state' for state JSON, got %q", result)
	}
}
