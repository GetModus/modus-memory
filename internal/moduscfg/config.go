package moduscfg

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// CartridgeConfig defines one office assignment in MODUS.
type CartridgeConfig struct {
	Role     string `yaml:"role"`
	Provider string `yaml:"provider,omitempty"`
	Family   string `yaml:"family"`
	Model    string `yaml:"model"`
	Backend  string `yaml:"backend"`
}

// MainBrainConfig defines the active sovereign cartridge for MODUS.
type MainBrainConfig = CartridgeConfig

// OfficersConfig holds the standing specialist offices.
type OfficersConfig struct {
	Librarian CartridgeConfig `yaml:"librarian"`
	Coder     CartridgeConfig `yaml:"coder"`
	Inspector CartridgeConfig `yaml:"inspector"`
	Scout     CartridgeConfig `yaml:"scout"`
}

// Config is the persisted ~/.modus/config.yaml shape relevant to runtime.
type Config struct {
	ProjectName string          `yaml:"project_name"`
	ProjectDir  string          `yaml:"project_dir"`
	OS          string          `yaml:"os"`
	Arch        string          `yaml:"arch"`
	CPUs        int             `yaml:"cpus"`
	TrustStage  int             `yaml:"trust_stage"`
	MainBrain   MainBrainConfig `yaml:"main_brain"`
	Officers    OfficersConfig  `yaml:"officers"`
}

type OfficeOption struct {
	Label      string
	Assignment CartridgeConfig
}

type ProviderCatalog struct {
	Provider string
	Family   string
	Backend  string
	Models   []string
}

const (
	defaultOllamaLibrarianModel = "gemma4:26b"
	defaultOllamaCoderModel     = "gemma4:31b"
	defaultMLXLibrarianModel    = "mlx-community/gemma-4-26b-a4b-it-4bit"
	defaultMLXCoderModel        = "mlx-community/gemma-4-31b-it-8bit"
)

var (
	liveOllamaOnce   sync.Once
	liveOllamaModels []string
	liveMLXOnce      sync.Once
	liveMLXModels    []string
	mlxHTTPClient    = &http.Client{Timeout: 750 * time.Millisecond}
	ollamaHTTPClient = &http.Client{Timeout: 750 * time.Millisecond}
)

// DefaultAssignment returns the default staffing for one office.
func DefaultAssignment(role string) CartridgeConfig {
	switch role {
	case "librarian":
		return CartridgeConfig{Role: "librarian", Provider: "ollama", Family: "local", Model: defaultOllamaModelForRole("librarian"), Backend: "ollama"}
	case "coder":
		return CartridgeConfig{Role: "coder", Provider: "ollama", Family: "local", Model: defaultOllamaModelForRole("coder"), Backend: "ollama"}
	case "inspector":
		return CartridgeConfig{Role: "inspector", Provider: "qwen", Family: "cloud", Model: "qwen-3.6", Backend: "cli"}
	case "scout":
		return CartridgeConfig{Role: "scout", Provider: "qwen", Family: "cloud", Model: "qwen-3.6", Backend: "cli"}
	default:
		return CartridgeConfig{Role: "modus", Provider: "openai", Family: "openai", Model: "chatgpt", Backend: "sdk"}
	}
}

func OfficeDisplayName(role string) string {
	switch role {
	case "main_brain":
		return "Commanding Officer"
	case "librarian":
		return "Librarian"
	case "coder":
		return "Minor Coder"
	case "inspector":
		return "Inspector"
	case "scout":
		return "Scout"
	default:
		return role
	}
}

func ProviderCatalogs() []ProviderCatalog {
	ollamaModels := providerCatalogOllamaModels()
	mlxModels := providerCatalogMLXModels()
	return []ProviderCatalog{
		{Provider: "openai", Family: "openai", Backend: "sdk", Models: []string{"chatgpt", "gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano"}},
		{Provider: "google", Family: "cloud", Backend: "api", Models: []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"}},
		{Provider: "qwen", Family: "cloud", Backend: "api", Models: []string{"qwen-plus", "qwen-max", "qwen3-235b-a22b"}},
		{Provider: "moonshot", Family: "cloud", Backend: "api", Models: []string{"kimi-k2.5", "kimi-k2-thinking", "kimi-k2-turbo-preview"}},
		{Provider: "minimax", Family: "cloud", Backend: "api", Models: []string{"MiniMax-M2.5", "MiniMax-M2.5-highspeed", "MiniMax-M2.1"}},
		{Provider: "nvidia", Family: "cloud", Backend: "api", Models: []string{"minimaxai/minimax-m2.7", "deepseek-ai/deepseek-v3_1-terminus", "mistralai/mistral-nemotron"}},
		{Provider: "ollama", Family: "local", Backend: "ollama", Models: ollamaModels},
		{Provider: "mlx", Family: "local", Backend: "api", Models: mlxModels},
	}
}

func RecommendedAssignments(role string) []OfficeOption {
	var options []OfficeOption
	add := func(label string, assignment CartridgeConfig) {
		assignment = applyDefaultAssignment(assignment, role)
		options = append(options, OfficeOption{Label: label, Assignment: assignment})
	}

	switch role {
	case "main_brain":
		add("OpenAI ChatGPT auth (recommended)", CartridgeConfig{Role: "modus", Provider: "openai", Family: "openai", Model: "chatgpt", Backend: "sdk"})
		add("OpenAI GPT-5.4", CartridgeConfig{Role: "modus", Provider: "openai", Family: "openai", Model: "gpt-5.4", Backend: "sdk"})
		add("Google Gemini 2.5 Pro", CartridgeConfig{Role: "modus", Provider: "google", Family: "cloud", Model: "gemini-2.5-pro", Backend: "api"})
		add("Moonshot Kimi K2.5", CartridgeConfig{Role: "modus", Provider: "moonshot", Family: "cloud", Model: "kimi-k2.5", Backend: "api"})
		add("MiniMax M2.5", CartridgeConfig{Role: "modus", Provider: "minimax", Family: "cloud", Model: "MiniMax-M2.5", Backend: "api"})
	case "librarian":
		add("Local Ollama runtime (recommended)", CartridgeConfig{Role: "librarian", Provider: "ollama", Family: "local", Model: defaultOllamaModelForRole("librarian"), Backend: "ollama"})
		add("Local MLX runtime", CartridgeConfig{Role: "librarian", Provider: "mlx", Family: "local", Model: defaultMLXModelForRole("librarian"), Backend: "api"})
		add("OpenAI GPT-5.4 mini", CartridgeConfig{Role: "librarian", Provider: "openai", Family: "openai", Model: "gpt-5.4-mini", Backend: "sdk"})
		add("Google Gemini 2.5 Flash", CartridgeConfig{Role: "librarian", Provider: "google", Family: "cloud", Model: "gemini-2.5-flash", Backend: "api"})
		add("Qwen Plus", CartridgeConfig{Role: "librarian", Provider: "qwen", Family: "cloud", Model: "qwen-plus", Backend: "api"})
		add("Moonshot Kimi K2.5", CartridgeConfig{Role: "librarian", Provider: "moonshot", Family: "cloud", Model: "kimi-k2.5", Backend: "api"})
	case "coder":
		add("Local Ollama runtime (recommended)", CartridgeConfig{Role: "coder", Provider: "ollama", Family: "local", Model: defaultOllamaModelForRole("coder"), Backend: "ollama"})
		add("Local MLX runtime", CartridgeConfig{Role: "coder", Provider: "mlx", Family: "local", Model: defaultMLXModelForRole("coder"), Backend: "api"})
		add("OpenAI GPT-5.4 mini", CartridgeConfig{Role: "coder", Provider: "openai", Family: "openai", Model: "gpt-5.4-mini", Backend: "sdk"})
		add("Google Gemini 2.5 Pro", CartridgeConfig{Role: "coder", Provider: "google", Family: "cloud", Model: "gemini-2.5-pro", Backend: "api"})
		add("Qwen 235B", CartridgeConfig{Role: "coder", Provider: "qwen", Family: "cloud", Model: "qwen3-235b-a22b", Backend: "api"})
		add("Moonshot Kimi K2.5", CartridgeConfig{Role: "coder", Provider: "moonshot", Family: "cloud", Model: "kimi-k2.5", Backend: "api"})
		add("MiniMax M2.5", CartridgeConfig{Role: "coder", Provider: "minimax", Family: "cloud", Model: "MiniMax-M2.5", Backend: "api"})
		add("Ollama Cloud GLM 5.1", CartridgeConfig{Role: "coder", Provider: "ollama", Family: "cloud", Model: "glm-5.1:cloud", Backend: "ollama"})
		add("NVIDIA MiniMax M2.7", CartridgeConfig{Role: "coder", Provider: "nvidia", Family: "cloud", Model: "minimaxai/minimax-m2.7", Backend: "api"})
	case "inspector":
		add("Qwen Plus API (recommended)", CartridgeConfig{Role: "inspector", Provider: "qwen", Family: "cloud", Model: "qwen-plus", Backend: "api"})
		add("Qwen CLI", CartridgeConfig{Role: "inspector", Provider: "qwen", Family: "cloud", Model: "qwen-3.6", Backend: "cli"})
		add("OpenAI GPT-5.4", CartridgeConfig{Role: "inspector", Provider: "openai", Family: "openai", Model: "gpt-5.4", Backend: "sdk"})
		add("Google Gemini 2.5 Flash", CartridgeConfig{Role: "inspector", Provider: "google", Family: "cloud", Model: "gemini-2.5-flash", Backend: "api"})
		add("MiniMax M2.5", CartridgeConfig{Role: "inspector", Provider: "minimax", Family: "cloud", Model: "MiniMax-M2.5", Backend: "api"})
		add("NVIDIA DeepSeek V3.1 Terminus", CartridgeConfig{Role: "inspector", Provider: "nvidia", Family: "cloud", Model: "deepseek-ai/deepseek-v3_1-terminus", Backend: "api"})
	case "scout":
		add("Qwen CLI (recommended)", CartridgeConfig{Role: "scout", Provider: "qwen", Family: "cloud", Model: "qwen-3.6", Backend: "cli"})
		add("Google Gemini 2.5 Flash", CartridgeConfig{Role: "scout", Provider: "google", Family: "cloud", Model: "gemini-2.5-flash", Backend: "api"})
		add("Google Gemini 2.5 Pro", CartridgeConfig{Role: "scout", Provider: "google", Family: "cloud", Model: "gemini-2.5-pro", Backend: "api"})
		add("OpenAI GPT-5.4 mini", CartridgeConfig{Role: "scout", Provider: "openai", Family: "openai", Model: "gpt-5.4-mini", Backend: "sdk"})
		add("Qwen Plus", CartridgeConfig{Role: "scout", Provider: "qwen", Family: "cloud", Model: "qwen-plus", Backend: "api"})
		add("Moonshot Kimi K2.5", CartridgeConfig{Role: "scout", Provider: "moonshot", Family: "cloud", Model: "kimi-k2.5", Backend: "api"})
		add("MiniMax M2.1", CartridgeConfig{Role: "scout", Provider: "minimax", Family: "cloud", Model: "MiniMax-M2.1", Backend: "api"})
		add("NVIDIA DeepSeek V3.1 Terminus", CartridgeConfig{Role: "scout", Provider: "nvidia", Family: "cloud", Model: "deepseek-ai/deepseek-v3_1-terminus", Backend: "api"})
	}
	return options
}

func ProviderModels(provider string) []string {
	for _, cat := range ProviderCatalogs() {
		if cat.Provider == provider {
			return append([]string(nil), cat.Models...)
		}
	}
	return nil
}

func NormalizeAssignment(role string, cfg CartridgeConfig) CartridgeConfig {
	return applyDefaultAssignment(cfg, role)
}

func FamilyForProvider(provider string) string {
	for _, cat := range ProviderCatalogs() {
		if cat.Provider == provider {
			return cat.Family
		}
	}
	return ""
}

func BackendForProvider(provider string) string {
	for _, cat := range ProviderCatalogs() {
		if cat.Provider == provider {
			return cat.Backend
		}
	}
	return ""
}

// DefaultConfig returns the commissioned default runtime config for a project.
func DefaultConfig(projectDir string) *Config {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		projectDir = "."
	}
	if abs, err := filepath.Abs(projectDir); err == nil {
		projectDir = abs
	}
	projectName := filepath.Base(projectDir)
	if projectName == "." || projectName == string(filepath.Separator) || strings.TrimSpace(projectName) == "" {
		projectName = "modus"
	}
	return &Config{
		ProjectName: projectName,
		ProjectDir:  projectDir,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		CPUs:        runtime.NumCPU(),
		TrustStage:  1,
		MainBrain:   DefaultAssignment("main_brain"),
		Officers: OfficersConfig{
			Librarian: DefaultAssignment("librarian"),
			Coder:     DefaultAssignment("coder"),
			Inspector: DefaultAssignment("inspector"),
			Scout:     DefaultAssignment("scout"),
		},
	}
}

func applyDefaultAssignment(cfg CartridgeConfig, role string) CartridgeConfig {
	def := DefaultAssignment(role)
	if cfg.Role == "" {
		cfg.Role = def.Role
	}
	if cfg.Provider == "" {
		cfg.Provider = inferProvider(cfg, def)
	}
	if cfg.Family == "" {
		cfg.Family = def.Family
	}
	if cfg.Model == "" {
		cfg.Model = def.Model
	}
	if cfg.Backend == "" {
		cfg.Backend = def.Backend
	}
	return cfg
}

func inferProvider(cfg CartridgeConfig, def CartridgeConfig) string {
	switch {
	case cfg.Provider != "":
		return cfg.Provider
	case cfg.Model == "chatgpt" || strings.HasPrefix(cfg.Model, "gpt-"):
		return "openai"
	case strings.HasPrefix(cfg.Model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(cfg.Model, "gemini-"):
		return "google"
	case strings.HasPrefix(cfg.Model, "mistral") || strings.HasPrefix(cfg.Model, "devstral"):
		return "mistral"
	case strings.HasPrefix(cfg.Model, "command-"):
		return "cohere"
	case strings.HasPrefix(cfg.Model, "deepseek-"):
		return "deepseek"
	case strings.HasPrefix(strings.ToLower(cfg.Model), "kimi-"):
		return "moonshot"
	case strings.HasPrefix(strings.ToLower(cfg.Model), "minimax-"):
		return "minimax"
	case strings.HasPrefix(strings.ToLower(cfg.Model), "minimaxai/"),
		strings.HasPrefix(strings.ToLower(cfg.Model), "deepseek-ai/"),
		strings.HasPrefix(strings.ToLower(cfg.Model), "mistralai/"):
		return "nvidia"
	case strings.HasPrefix(strings.ToLower(cfg.Model), "gpt-oss-"),
		strings.HasPrefix(strings.ToLower(cfg.Model), "zai-glm-"),
		strings.HasPrefix(strings.ToLower(cfg.Model), "qwen-3-235b-a22b-instruct-"):
		return "cerebras"
	case strings.HasPrefix(cfg.Model, "mlx-community/"):
		return "mlx"
	case strings.Contains(cfg.Model, "qwen"):
		return "qwen"
	case cfg.Backend == "ollama":
		return "ollama"
	default:
		return def.Provider
	}
}

// DefaultPath returns ~/.modus/config.yaml.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".modus", "config.yaml")
}

// Load reads the MODUS config file from disk.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.MainBrain = applyDefaultAssignment(cfg.MainBrain, "main_brain")
	cfg.Officers.Librarian = applyDefaultAssignment(cfg.Officers.Librarian, "librarian")
	cfg.Officers.Coder = applyDefaultAssignment(cfg.Officers.Coder, "coder")
	cfg.Officers.Inspector = applyDefaultAssignment(cfg.Officers.Inspector, "inspector")
	cfg.Officers.Scout = applyDefaultAssignment(cfg.Officers.Scout, "scout")
	return &cfg, nil
}

// LoadDefault reads ~/.modus/config.yaml if it exists.
func LoadDefault() (*Config, error) {
	return Load(DefaultPath())
}

// LoadOrCreateDefault loads ~/.modus/config.yaml or seeds it from the commissioned defaults.
func LoadOrCreateDefault() (*Config, error) {
	path := DefaultPath()
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	wd, wdErr := os.Getwd()
	if wdErr != nil {
		wd = "."
	}
	cfg = DefaultConfig(wd)
	if err := Save(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func defaultMLXModelForRole(role string) string {
	models := liveMLXCatalog()
	if len(models) > 0 {
		if role == "coder" && len(models) > 1 {
			return models[1]
		}
		return models[0]
	}
	if role == "coder" {
		return defaultMLXCoderModel
	}
	return defaultMLXLibrarianModel
}

func defaultOllamaModelForRole(role string) string {
	models := liveOllamaCatalog()
	if len(models) > 0 {
		if role == "coder" {
			for _, model := range models {
				if strings.EqualFold(strings.TrimSpace(model), defaultOllamaCoderModel) {
					return model
				}
			}
		}
		for _, model := range models {
			if strings.EqualFold(strings.TrimSpace(model), defaultOllamaLibrarianModel) {
				return model
			}
		}
		if role == "coder" && len(models) > 1 {
			return models[1]
		}
		return models[0]
	}
	if role == "coder" {
		return defaultOllamaCoderModel
	}
	return defaultOllamaLibrarianModel
}

func providerCatalogOllamaModels() []string {
	models := liveOllamaCatalog()
	if len(models) > 0 {
		return models
	}
	return []string{defaultOllamaLibrarianModel, defaultOllamaCoderModel}
}

func providerCatalogMLXModels() []string {
	models := liveMLXCatalog()
	if len(models) > 0 {
		return models
	}
	return []string{defaultMLXLibrarianModel, defaultMLXCoderModel}
}

func liveOllamaCatalog() []string {
	liveOllamaOnce.Do(func() {
		liveOllamaModels = fetchLiveOllamaModels()
	})
	return append([]string(nil), liveOllamaModels...)
}

func liveMLXCatalog() []string {
	liveMLXOnce.Do(func() {
		liveMLXModels = fetchLiveMLXModels()
	})
	return append([]string(nil), liveMLXModels...)
}

func fetchLiveOllamaModels() []string {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("OLLAMA_HOST")), "/")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	req, err := http.NewRequest("GET", base+"/api/tags", nil)
	if err != nil {
		return nil
	}
	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	var models []string
	seen := map[string]bool{}
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		models = append(models, model)
	}
	for _, item := range payload.Models {
		add(item.Name)
	}
	return models
}

func fetchLiveMLXModels() []string {
	base := strings.TrimSuffix(os.Getenv("MODUS_MLX_SERVER_URL"), "/")
	if base == "" {
		base = "http://127.0.0.1:8090/v1"
	}
	req, err := http.NewRequest("GET", base+"/models", nil)
	if err != nil {
		return nil
	}
	resp, err := mlxHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	var models []string
	seen := map[string]bool{}
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		models = append(models, model)
	}
	for _, item := range payload.Data {
		add(item.ID)
	}
	for _, item := range payload.Models {
		add(item.Name)
	}
	return models
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func staticJSONClient(status int, body string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}
