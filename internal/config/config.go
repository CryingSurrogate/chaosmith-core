package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config holds runtime configuration for Chaosmith Core.
// Values map to PCS/1.3-native environment knobs and can be overridden by env vars.
type Config struct {
	SurrealURL  string `toml:"surreal_url"`
	SurrealUser string `toml:"surreal_user"`
	SurrealPass string `toml:"surreal_pass"`
	SurrealNS   string `toml:"surreal_ns"`
	SurrealDB   string `toml:"surreal_db"`

	EmbedKind     string `toml:"embed_kind"`
	EmbedURL      string `toml:"embed_url"`
	EmbedModel    string `toml:"embed_model"`
	EmbedModelSHA string `toml:"embed_model_sha"`
	EffectiveDim  int    `toml:"effective_dim"`
	TransformID   string `toml:"transform_id"`
	TokenizerID   string `toml:"tokenizer_id"`

	ArtifactRoot string   `toml:"artifact_root"`
	WorkspaceIDs []string `toml:"work_roots"`

	IndexerBinary string `toml:"indexer_bin"`
	CTagsPath     string `toml:"ctags_path"`
}

// Load reads configuration from the provided path, applying environment overrides.
func Load(path string) (*Config, error) {
	cfg := &Config{
		ArtifactRoot: "var/lib/chaosmith/artifacts",
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	applyEnvOverrides(cfg)
	normalize(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	set := func(dst *string, env string) {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			*dst = v
		}
	}
	set(&cfg.SurrealURL, "SURREAL_URL")
	set(&cfg.SurrealUser, "SURREAL_USER")
	set(&cfg.SurrealPass, "SURREAL_PASS")
	set(&cfg.SurrealNS, "SURREAL_NS")
	set(&cfg.SurrealDB, "SURREAL_DB")

	set(&cfg.EmbedKind, "EMBED_KIND")
	set(&cfg.EmbedURL, "EMBED_URL")
	set(&cfg.EmbedModel, "EMBED_MODEL")
	set(&cfg.EmbedModelSHA, "EMBED_MODEL_SHA")
	set(&cfg.TransformID, "TRANSFORM_ID")
	set(&cfg.TokenizerID, "TOKENIZER_ID")

	if v := strings.TrimSpace(os.Getenv("EFFECTIVE_DIM")); v != "" {
		if dim, err := parseInt(v); err == nil {
			cfg.EffectiveDim = dim
		}
	}

	if v := strings.TrimSpace(os.Getenv("WORK_ROOTS")); v != "" {
		cfg.WorkspaceIDs = splitCSV(v)
	}
	set(&cfg.ArtifactRoot, "ARTIFACT_ROOT")
	set(&cfg.IndexerBinary, "INDEXER_BIN")
	set(&cfg.CTagsPath, "CTAGS_PATH")
}

func normalize(cfg *Config) {
	cfg.SurrealURL = strings.TrimSpace(cfg.SurrealURL)
	cfg.SurrealUser = strings.TrimSpace(cfg.SurrealUser)
	cfg.SurrealPass = strings.TrimSpace(cfg.SurrealPass)
	cfg.SurrealNS = strings.TrimSpace(cfg.SurrealNS)
	cfg.SurrealDB = strings.TrimSpace(cfg.SurrealDB)

	cfg.EmbedKind = strings.ToLower(strings.TrimSpace(cfg.EmbedKind))
	cfg.EmbedURL = strings.TrimSpace(cfg.EmbedURL)
	cfg.EmbedModel = strings.TrimSpace(cfg.EmbedModel)
	cfg.EmbedModelSHA = strings.TrimSpace(cfg.EmbedModelSHA)
	cfg.TransformID = strings.TrimSpace(cfg.TransformID)
	cfg.TokenizerID = strings.TrimSpace(cfg.TokenizerID)

	cfg.ArtifactRoot = filepath.Clean(cfg.ArtifactRoot)
	cfg.IndexerBinary = strings.TrimSpace(cfg.IndexerBinary)
	cfg.CTagsPath = strings.TrimSpace(cfg.CTagsPath)
}

func validate(cfg *Config) error {
	var missing []string

	if cfg.SurrealURL == "" {
		missing = append(missing, "surreal_url")
	}
	if cfg.SurrealNS == "" {
		missing = append(missing, "surreal_ns")
	}
	if cfg.SurrealDB == "" {
		missing = append(missing, "surreal_db")
	}
	if cfg.EmbedURL == "" {
		missing = append(missing, "embed_url")
	}
	if cfg.EmbedModel == "" {
		missing = append(missing, "embed_model")
	}
	if cfg.EmbedModelSHA == "" {
		missing = append(missing, "embed_model_sha")
	}
	if cfg.EffectiveDim == 0 {
		missing = append(missing, "effective_dim")
	}
	if cfg.TransformID == "" {
		missing = append(missing, "transform_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config missing required fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

func parseInt(v string) (int, error) {
	var out int
	_, err := fmt.Sscanf(v, "%d", &out)
	return out, err
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	var out []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ErrToolMissing is returned when a required external tool is unavailable.
var ErrToolMissing = errors.New("tool missing")
