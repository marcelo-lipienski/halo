package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseEnv(t *testing.T) {
	// Create a temporary env file
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	envContent := `
# A comment line
DB_HOST=localhost # Hostname
DB_PORT=5432
DB_USER="postgres" # Inline comment after double quote
DB_PASS='secret' # Inline comment after single quote
# Another comment
EMPTY_VAR=
COMMENT_VAR="secret#key" # Comment inside quotes
UNQUOTED_COMMENT=foo #bar
UNQUOTED_NO_COMMENT=foo#bar
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}

	expected := map[string]string{
		"DB_HOST":             "localhost",
		"DB_PORT":             "5432",
		"DB_USER":             "postgres",
		"DB_PASS":             "secret",
		"EMPTY_VAR":           "",
		"COMMENT_VAR":         "secret#key",
		"UNQUOTED_COMMENT":    "foo",
		"UNQUOTED_NO_COMMENT": "foo#bar",
	}

	result, err := ParseEnv(envPath)
	if err != nil {
		t.Fatalf("unexpected error parsing env: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestParseCompose(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    environment:
      - PORT=8080
      - DEBUG
    ports:
      - "80:8080"
    volumes:
      - ./html:/var/www/html
      - data-volume:/data
  db:
    environment:
      POSTGRES_DB: main
      POSTGRES_USER: admin
    ports:
      - "5432:5432"
    volumes:
      - type: bind
        source: ./db_data
        target: /var/lib/postgresql/data
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose file: %v", err)
	}

	config, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error parsing compose: %v", err)
	}

	// Verify web service
	web, ok := config.Services["web"]
	if !ok {
		t.Fatal("web service not found")
	}

	expectedWebEnv := ComposeEnvironment{
		"PORT":  "8080",
		"DEBUG": "",
	}
	if !reflect.DeepEqual(web.Environment, expectedWebEnv) {
		t.Errorf("web env expected %v, got %v", expectedWebEnv, web.Environment)
	}

	if len(web.Ports) != 1 || web.Ports[0] != "80:8080" {
		t.Errorf("unexpected web ports: %v", web.Ports)
	}

	if len(web.Volumes) != 2 {
		t.Fatalf("expected 2 web volumes, got %d", len(web.Volumes))
	}

	if web.Volumes[0].Source != "./html" || web.Volumes[0].Target != "/var/www/html" || web.Volumes[0].Type != "bind" {
		t.Errorf("unexpected web volume 0: %+v", web.Volumes[0])
	}

	if web.Volumes[1].Source != "data-volume" || web.Volumes[1].Target != "/data" || web.Volumes[1].Type != "volume" {
		t.Errorf("unexpected web volume 1: %+v", web.Volumes[1])
	}

	// Verify db service
	db, ok := config.Services["db"]
	if !ok {
		t.Fatal("db service not found")
	}

	expectedDbEnv := ComposeEnvironment{
		"POSTGRES_DB":   "main",
		"POSTGRES_USER": "admin",
	}
	if !reflect.DeepEqual(db.Environment, expectedDbEnv) {
		t.Errorf("db env expected %v, got %v", expectedDbEnv, db.Environment)
	}

	if len(db.Volumes) != 1 {
		t.Fatalf("expected 1 db volume, got %d", len(db.Volumes))
	}

	if db.Volumes[0].Source != "./db_data" || db.Volumes[0].Target != "/var/lib/postgresql/data" || db.Volumes[0].Type != "bind" {
		t.Errorf("unexpected db volume: %+v", db.Volumes[0])
	}
}

func TestStringOrSlice(t *testing.T) {
	composeContent := `
services:
  web:
    entrypoint: /bin/sh
    command: ["-c", "echo hello"]
`
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose file: %v", err)
	}

	config, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error parsing compose: %v", err)
	}

	web, ok := config.Services["web"]
	if !ok {
		t.Fatal("web service not found")
	}

	expectedEntrypoint := StringOrSlice{"/bin/sh"}
	if !reflect.DeepEqual(web.Entrypoint, expectedEntrypoint) {
		t.Errorf("entrypoint expected %v, got %v", expectedEntrypoint, web.Entrypoint)
	}

	expectedCommand := StringOrSlice{"-c", "echo hello"}
	if !reflect.DeepEqual(web.Command, expectedCommand) {
		t.Errorf("command expected %v, got %v", expectedCommand, web.Command)
	}
}

func TestWindowsVolumeParsing(t *testing.T) {
	composeContent := `
services:
  web:
    volumes:
      - C:\host\data:/container/data
      - D:/another/path:/container/another:ro
`
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose file: %v", err)
	}

	config, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error parsing compose: %v", err)
	}

	web, ok := config.Services["web"]
	if !ok {
		t.Fatal("web service not found")
	}

	if len(web.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(web.Volumes))
	}

	if web.Volumes[0].Source != `C:\host\data` || web.Volumes[0].Target != `/container/data` || web.Volumes[0].Type != "bind" {
		t.Errorf("unexpected volume 0: %+v", web.Volumes[0])
	}

	if web.Volumes[1].Source != `D:/another/path` || web.Volumes[1].Target != `/container/another` || web.Volumes[1].Type != "bind" {
		t.Errorf("unexpected volume 1: %+v", web.Volumes[1])
	}
}
