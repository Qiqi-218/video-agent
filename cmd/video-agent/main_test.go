package main

import (
	"context"
	"testing"
)

func TestToolListAndProjectNameFlagsDoNotConflict(t *testing.T) {
	data := t.TempDir()
	if _, err := run(context.Background(), []string{"--data", data, "tool", "list"}); err != nil {
		t.Fatalf("tool list: %v", err)
	}
	if _, err := run(context.Background(), []string{"--data", data, "project", "create", "--id", "p", "--name", "project name"}); err != nil {
		t.Fatalf("project create: %v", err)
	}
}

func TestServeRejectsNonLoopbackUnlessContainerOverride(t *testing.T) {
	if err := loopback("0.0.0.0:8090"); err == nil {
		t.Fatal("unrestricted listen address was accepted")
	}
	t.Setenv("VIDEO_AGENT_ALLOW_CONTAINER_LISTEN", "1")
	if err := loopback("0.0.0.0:8090"); err != nil {
		t.Fatalf("container override: %v", err)
	}
}
