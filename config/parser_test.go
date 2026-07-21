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

func TestParseEnvWithExport(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	envContent := `
export DB_HOST=localhost
export DB_PORT=5432
DB_USER=postgres
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}

	expected := map[string]string{
		"DB_HOST": "localhost",
		"DB_PORT": "5432",
		"DB_USER": "postgres",
	}

	result, err := ParseEnv(envPath)
	if err != nil {
		t.Fatalf("unexpected error parsing env: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestStructuredPortsParsingAndReadOnlyVolumes(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    ports:
      - target: 80
        published: 8080
        protocol: tcp
        mode: host
      - "443:443"
    volumes:
      - ./data:/var/data:ro
      - type: bind
        source: ./config
        target: /app/config
        read_only: true
`
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

	expectedPorts := ComposePorts{"8080:80/tcp", "443:443"}
	if !reflect.DeepEqual(web.Ports, expectedPorts) {
		t.Errorf("expected ports %v, got %v", expectedPorts, web.Ports)
	}

	if len(web.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(web.Volumes))
	}

	if !web.Volumes[0].ReadOnly || web.Volumes[0].Source != "./data" {
		t.Errorf("expected volume 0 to be read-only, got %+v", web.Volumes[0])
	}

	if !web.Volumes[1].ReadOnly || web.Volumes[1].Source != "./config" {
		t.Errorf("expected volume 1 to be read-only, got %+v", web.Volumes[1])
	}
}

func TestMergeComposeConfigs(t *testing.T) {
	cfg1 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Image: "nginx:latest",
				Environment: ComposeEnvironment{
					"PORT":  "80",
					"DEBUG": "true",
				},
				Ports: ComposePorts{"80:80"},
				Volumes: []ComposeVolume{
					{Source: "./data", Target: "/data", Type: "bind"},
				},
			},
		},
		Volumes: map[string]interface{}{
			"shared-data": nil,
		},
	}

	cfg2 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Image: "nginx:alpine",
				Environment: ComposeEnvironment{
					"DEBUG":   "false",
					"NEW_VAR": "value",
				},
				Ports: ComposePorts{"443:443"},
				Volumes: []ComposeVolume{
					{Source: "./config", Target: "/config", Type: "bind"},
				},
			},
			"db": {
				Image: "postgres:latest",
			},
		},
		Volumes: map[string]interface{}{
			"db-data": nil,
		},
	}

	merged := MergeComposeConfigs(cfg1, cfg2)

	web := merged.Services["web"]
	if web.Image != "nginx:alpine" {
		t.Errorf("expected image nginx:alpine, got %s", web.Image)
	}

	expectedEnv := ComposeEnvironment{
		"PORT":    "80",
		"DEBUG":   "false",
		"NEW_VAR": "value",
	}
	if !reflect.DeepEqual(web.Environment, expectedEnv) {
		t.Errorf("expected environment %v, got %v", expectedEnv, web.Environment)
	}

	expectedPorts := ComposePorts{"80:80", "443:443"}
	if !reflect.DeepEqual(web.Ports, expectedPorts) {
		t.Errorf("expected ports %v, got %v", expectedPorts, web.Ports)
	}

	if len(web.Volumes) != 2 {
		t.Errorf("expected 2 volumes, got %d", len(web.Volumes))
	}

	if _, ok := merged.Services["db"]; !ok {
		t.Error("expected db service to be merged in")
	}

	if _, ok := merged.Volumes["shared-data"]; !ok {
		t.Error("expected shared-data root volume to exist")
	}
	if _, ok := merged.Volumes["db-data"]; !ok {
		t.Error("expected db-data root volume to exist")
	}
}

func TestMergeComposeConfigsOverlappingVolumes(t *testing.T) {
	cfg1 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Volumes: []ComposeVolume{
					{Source: "./data-old", Target: "/data", Type: "bind"},
					{Source: "./other", Target: "/other", Type: "bind"},
				},
			},
		},
	}

	cfg2 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Volumes: []ComposeVolume{
					{Source: "./data-new", Target: "/data", Type: "bind"},
				},
			},
		},
	}

	merged := MergeComposeConfigs(cfg1, cfg2)
	web := merged.Services["web"]

	if len(web.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(web.Volumes))
	}

	// First volume should be "./other" -> "/other"
	if web.Volumes[0].Source != "./other" || web.Volumes[0].Target != "/other" {
		t.Errorf("expected first volume to be ./other, got %+v", web.Volumes[0])
	}

	// Second volume should be "./data-new" -> "/data" (overridden)
	if web.Volumes[1].Source != "./data-new" || web.Volumes[1].Target != "/data" {
		t.Errorf("expected second volume to be ./data-new, got %+v", web.Volumes[1])
	}
}

func TestParseSecretsAndConfigs(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
secrets:
  my_secret:
    file: ./secret.txt
  ext_secret:
    external: true
configs:
  my_config:
    file: ./config.txt
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose: %v", err)
	}

	cfg, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Secrets) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(cfg.Secrets))
	}
	sec, ok := cfg.Secrets["my_secret"]
	if !ok || sec.File != "./secret.txt" {
		t.Errorf("unexpected secret configuration: %+v", sec)
	}

	if len(cfg.Configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(cfg.Configs))
	}
	conf, ok := cfg.Configs["my_config"]
	if !ok || conf.File != "./config.txt" {
		t.Errorf("unexpected config configuration: %+v", conf)
	}
}

func TestAnonymousVolumeParsing(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    volumes:
      - /var/lib/mysql
      - named-vol:/var/lib/other
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose: %v", err)
	}

	cfg, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	web := cfg.Services["web"]
	if len(web.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(web.Volumes))
	}

	// First should be anonymous volume (type: volume, empty source, target /var/lib/mysql)
	if web.Volumes[0].Type != "volume" || web.Volumes[0].Source != "" || web.Volumes[0].Target != "/var/lib/mysql" {
		t.Errorf("unexpected volume 0: %+v", web.Volumes[0])
	}

	// Second should be named volume (type: volume, source named-vol, target /var/lib/other)
	if web.Volumes[1].Type != "volume" || web.Volumes[1].Source != "named-vol" || web.Volumes[1].Target != "/var/lib/other" {
		t.Errorf("unexpected volume 1: %+v", web.Volumes[1])
	}
}

func TestParseComposeNullEnv(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    environment:
      PORT: 8080
      NULL_VAR: null
      EMPTY_VAR: ""
`
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

	expectedWebEnv := ComposeEnvironment{
		"PORT":      "8080",
		"NULL_VAR":  "",
		"EMPTY_VAR": "",
	}
	if !reflect.DeepEqual(web.Environment, expectedWebEnv) {
		t.Errorf("web env expected %v, got %v", expectedWebEnv, web.Environment)
	}
}

func BenchmarkParseCompose(b *testing.B) {
	tempDir := b.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
    ports:
      - "80:80"
  db:
    image: postgres
    volumes:
      - ./data:/var/lib/postgresql/data
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		b.Fatalf("failed to write temp compose file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseCompose(composePath)
	}
}

func BenchmarkParseEnv(b *testing.B) {
	tempDir := b.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	envContent := `
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASS=secret
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		b.Fatalf("failed to write temp env file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseEnv(envPath)
	}
}

func TestParseEnvFiles(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web1:
    env_file: .env.one
  web2:
    env_file:
      - .env.common
      - .env.dev
  web3:
    env_file:
      - file: .env.prod
        required: true
      - file: .env.opt
        required: false
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose: %v", err)
	}

	cfg, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error parsing compose: %v", err)
	}

	// verify web1
	w1, ok := cfg.Services["web1"]
	if !ok {
		t.Fatal("web1 not found")
	}
	if len(w1.EnvFiles) != 1 || w1.EnvFiles[0].File != ".env.one" || !w1.EnvFiles[0].Required {
		t.Errorf("unexpected web1 env_file: %+v", w1.EnvFiles)
	}

	// verify web2
	w2, ok := cfg.Services["web2"]
	if !ok {
		t.Fatal("web2 not found")
	}
	if len(w2.EnvFiles) != 2 || w2.EnvFiles[0].File != ".env.common" || w2.EnvFiles[1].File != ".env.dev" {
		t.Errorf("unexpected web2 env_file: %+v", w2.EnvFiles)
	}

	// verify web3
	w3, ok := cfg.Services["web3"]
	if !ok {
		t.Fatal("web3 not found")
	}
	if len(w3.EnvFiles) != 2 {
		t.Fatalf("expected 2 env_files, got %d", len(w3.EnvFiles))
	}
	if w3.EnvFiles[0].File != ".env.prod" || !w3.EnvFiles[0].Required {
		t.Errorf("unexpected web3 env_file 0: %+v", w3.EnvFiles[0])
	}
	if w3.EnvFiles[1].File != ".env.opt" || w3.EnvFiles[1].Required {
		t.Errorf("unexpected web3 env_file 1: %+v", w3.EnvFiles[1])
	}
}

func TestMergeComposeConfigsWithEnvFiles(t *testing.T) {
	cfg1 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				EnvFiles: ComposeEnvFiles{
					{File: ".env.common", Required: true},
				},
			},
		},
	}
	cfg2 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				EnvFiles: ComposeEnvFiles{
					{File: ".env.override", Required: false},
				},
			},
		},
	}

	merged := MergeComposeConfigs(cfg1, cfg2)
	web := merged.Services["web"]
	if len(web.EnvFiles) != 2 {
		t.Fatalf("expected 2 env_files after merge, got %d", len(web.EnvFiles))
	}
	if web.EnvFiles[0].File != ".env.common" || web.EnvFiles[1].File != ".env.override" {
		t.Errorf("unexpected merged env_files: %+v", web.EnvFiles)
	}
}

func TestParseComposeSecretsConfigs(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    secrets:
      - secret_a
      - source: secret_b
        target: /run/secrets/b
    configs:
      - config_a
      - source: config_b
        target: /etc/config_b
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose: %v", err)
	}

	cfg, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error parsing compose: %v", err)
	}

	web, ok := cfg.Services["web"]
	if !ok {
		t.Fatal("web service not found")
	}

	if len(web.Secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(web.Secrets))
	}
	if web.Secrets[0].Source != "secret_a" {
		t.Errorf("expected secret_a, got %s", web.Secrets[0].Source)
	}
	if web.Secrets[1].Source != "secret_b" || web.Secrets[1].Target != "/run/secrets/b" {
		t.Errorf("unexpected secret_b layout: %+v", web.Secrets[1])
	}

	if len(web.Configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(web.Configs))
	}
	if web.Configs[0].Source != "config_a" {
		t.Errorf("expected config_a, got %s", web.Configs[0].Source)
	}
	if web.Configs[1].Source != "config_b" || web.Configs[1].Target != "/etc/config_b" {
		t.Errorf("unexpected config_b layout: %+v", web.Configs[1])
	}
}

func TestMergeComposeConfigsSecretsConfigs(t *testing.T) {
	cfg1 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Secrets: ComposeServiceSecrets{
					{Source: "secret_a", Target: "/run/secrets/a"},
				},
				Configs: ComposeServiceConfigs{
					{Source: "config_a", Target: "/etc/a"},
				},
			},
		},
	}
	cfg2 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Secrets: ComposeServiceSecrets{
					{Source: "secret_a", Target: "/run/secrets/a_new"},
					{Source: "secret_b"},
				},
				Configs: ComposeServiceConfigs{
					{Source: "config_a", Target: "/etc/a_new"},
					{Source: "config_b"},
				},
			},
		},
	}

	merged := MergeComposeConfigs(cfg1, cfg2)
	web := merged.Services["web"]

	// Verify secrets (should merge and overwrite by Source, sorted alphabetically)
	if len(web.Secrets) != 2 {
		t.Fatalf("expected 2 merged secrets, got %d", len(web.Secrets))
	}
	if web.Secrets[0].Source != "secret_a" || web.Secrets[0].Target != "/run/secrets/a_new" {
		t.Errorf("unexpected secret 0: %+v", web.Secrets[0])
	}
	if web.Secrets[1].Source != "secret_b" {
		t.Errorf("unexpected secret 1: %+v", web.Secrets[1])
	}

	// Verify configs (should merge and overwrite by Source, sorted alphabetically)
	if len(web.Configs) != 2 {
		t.Fatalf("expected 2 merged configs, got %d", len(web.Configs))
	}
	if web.Configs[0].Source != "config_a" || web.Configs[0].Target != "/etc/a_new" {
		t.Errorf("unexpected config 0: %+v", web.Configs[0])
	}
	if web.Configs[1].Source != "config_b" {
		t.Errorf("unexpected config 1: %+v", web.Configs[1])
	}
}

func TestParseComposeBuild(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web1:
    build: .
  web2:
    build:
      context: ./src
      dockerfile: Dockerfile.prod
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write temp compose: %v", err)
	}

	cfg, err := ParseCompose(composePath)
	if err != nil {
		t.Fatalf("unexpected error parsing compose: %v", err)
	}

	web1, ok := cfg.Services["web1"]
	if !ok {
		t.Fatal("web1 not found")
	}
	if web1.Build.Context != "." || web1.Build.Dockerfile != "" {
		t.Errorf("unexpected web1 build: %+v", web1.Build)
	}

	web2, ok := cfg.Services["web2"]
	if !ok {
		t.Fatal("web2 not found")
	}
	if web2.Build.Context != "./src" || web2.Build.Dockerfile != "Dockerfile.prod" {
		t.Errorf("unexpected web2 build: %+v", web2.Build)
	}

	// Test merging build block
	cfg1 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Build: ComposeBuild{Context: "."},
			},
		},
	}
	cfg2 := &ComposeConfig{
		Services: map[string]ComposeService{
			"web": {
				Build: ComposeBuild{Context: "./src", Dockerfile: "Dockerfile.dev"},
			},
		},
	}

	merged := MergeComposeConfigs(cfg1, cfg2)
	web := merged.Services["web"]
	if web.Build.Context != "./src" || web.Build.Dockerfile != "Dockerfile.dev" {
		t.Errorf("unexpected merged build: %+v", web.Build)
	}
}
