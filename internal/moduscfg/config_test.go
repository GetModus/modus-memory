package moduscfg

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoadMainBrainConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `project_name: "modus"
trust_stage: 2
main_brain:
  role: "modus"
  provider: "openai"
  family: "openai"
  model: "chatgpt"
  backend: "sdk"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.MainBrain.Role != "modus" {
		t.Fatalf("role = %q, want modus", cfg.MainBrain.Role)
	}
	if cfg.MainBrain.Model != "chatgpt" {
		t.Fatalf("model = %q, want chatgpt", cfg.MainBrain.Model)
	}
	if cfg.MainBrain.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", cfg.MainBrain.Provider)
	}
	if cfg.MainBrain.Backend != "sdk" {
		t.Fatalf("backend = %q, want sdk", cfg.MainBrain.Backend)
	}
}

func TestLoadDefaultsRoleWhenMissing(t *testing.T) {
	liveOllamaOnce = sync.Once{}
	liveOllamaModels = nil
	liveMLXOnce = sync.Once{}
	liveMLXModels = nil
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `trust_stage: 1
main_brain:
  family: "openai"
  model: "gpt-5.4"
  backend: "sdk"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.MainBrain.Role != "modus" {
		t.Fatalf("default role = %q, want modus", cfg.MainBrain.Role)
	}
	if cfg.MainBrain.Provider != "openai" {
		t.Fatalf("default provider = %q, want openai", cfg.MainBrain.Provider)
	}
	if cfg.Officers.Librarian.Model != defaultOllamaLibrarianModel {
		t.Fatalf("default librarian model = %q, want %s", cfg.Officers.Librarian.Model, defaultOllamaLibrarianModel)
	}
	if cfg.Officers.Coder.Backend != "ollama" {
		t.Fatalf("default coder backend = %q, want ollama", cfg.Officers.Coder.Backend)
	}
	if cfg.Officers.Inspector.Role != "inspector" {
		t.Fatalf("default inspector role = %q, want inspector", cfg.Officers.Inspector.Role)
	}
	if cfg.Officers.Scout.Model != "qwen-3.6" {
		t.Fatalf("default scout model = %q, want qwen-3.6", cfg.Officers.Scout.Model)
	}
}

func TestLoadDefaultsUseLiveLocalCatalogsWhenAvailable(t *testing.T) {
	oldOllamaClient := ollamaHTTPClient
	ollamaHTTPClient = staticJSONClient(200, `{"models":[{"name":"gemma4:26b"},{"name":"gemma4:31b"},{"name":"qwen2.5:14b"}]}`)
	defer func() { ollamaHTTPClient = oldOllamaClient }()
	oldClient := mlxHTTPClient
	mlxHTTPClient = staticJSONClient(200, `{"data":[{"id":"gemma-4-26B-A4B-it-UD-Q4_K_M.gguf"},{"id":"gemma-4-31B-it-Q8_0.gguf"}]}`)
	defer func() { mlxHTTPClient = oldClient }()

	t.Setenv("OLLAMA_HOST", "http://ollama.test")
	t.Setenv("MODUS_MLX_SERVER_URL", "http://mlx.test/v1")
	liveOllamaOnce = sync.Once{}
	liveOllamaModels = nil
	liveMLXOnce = sync.Once{}
	liveMLXModels = nil

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("trust_stage: 1\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Officers.Librarian.Model != "gemma4:26b" {
		t.Fatalf("live librarian model = %q", cfg.Officers.Librarian.Model)
	}
	if cfg.Officers.Coder.Model != "gemma4:31b" {
		t.Fatalf("live coder model = %q", cfg.Officers.Coder.Model)
	}
	if got := ProviderModels("ollama"); len(got) != 3 || !strings.Contains(strings.Join(got, ","), "gemma4:31b") {
		t.Fatalf("ollama provider models did not use live catalog: %+v", got)
	}
	if got := ProviderModels("mlx"); len(got) != 2 || !strings.Contains(strings.Join(got, ","), "gemma-4-31B-it-Q8_0.gguf") {
		t.Fatalf("mlx provider models did not use live catalog: %+v", got)
	}
}

func TestRecommendedAssignmentsMainBrainUsesCommandProfile(t *testing.T) {
	options := RecommendedAssignments("main_brain")
	if len(options) == 0 {
		t.Fatal("expected recommended assignments")
	}
	if options[0].Assignment.Provider == "" {
		t.Fatal("expected provider on recommended assignment")
	}
	if options[0].Assignment.Role != "modus" {
		t.Fatalf("role = %q, want modus", options[0].Assignment.Role)
	}
}

func TestProviderModelsIncludesExpandedCloudProviders(t *testing.T) {
	if got := ProviderModels("moonshot"); len(got) == 0 {
		t.Fatal("expected moonshot models")
	}
	if got := ProviderModels("minimax"); len(got) == 0 {
		t.Fatal("expected minimax models")
	}
	if got := ProviderModels("nvidia"); len(got) == 0 {
		t.Fatal("expected nvidia models")
	}
	if got := ProviderModels("qwen"); len(got) == 0 {
		t.Fatal("expected qwen models")
	}
	if got := ProviderModels("ollama"); len(got) == 0 {
		t.Fatal("expected ollama models")
	}
	if got := ProviderModels("mlx"); len(got) == 0 {
		t.Fatal("expected mlx models")
	}
}

func TestLoadOrCreateDefaultSeedsCommissionedConfig(t *testing.T) {
	home := t.TempDir()
	workdir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	liveOllamaOnce = sync.Once{}
	liveOllamaModels = nil
	liveMLXOnce = sync.Once{}
	liveMLXModels = nil

	cfg, err := LoadOrCreateDefault()
	if err != nil {
		t.Fatalf("LoadOrCreateDefault: %v", err)
	}
	if cfg.ProjectName != "workspace" {
		t.Fatalf("project name = %q, want workspace", cfg.ProjectName)
	}
	if cfg.Officers.Librarian.Provider != "ollama" || cfg.Officers.Librarian.Model != defaultOllamaLibrarianModel {
		t.Fatalf("librarian default = %+v", cfg.Officers.Librarian)
	}
	if cfg.Officers.Coder.Provider != "ollama" || cfg.Officers.Coder.Model != defaultOllamaCoderModel {
		t.Fatalf("coder default = %+v", cfg.Officers.Coder)
	}
	if cfg.Officers.Scout.Provider != "qwen" || cfg.Officers.Scout.Model != "qwen-3.6" || cfg.Officers.Scout.Backend != "cli" {
		t.Fatalf("scout default = %+v", cfg.Officers.Scout)
	}
	if _, err := os.Stat(DefaultPath()); err != nil {
		t.Fatalf("expected seeded config at %s: %v", DefaultPath(), err)
	}
}
