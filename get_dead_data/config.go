package main

import (
	"os"

	"github.com/BurntSushi/toml"
)

// ProfilesConfig mirrors the structure of profiles.toml
type ProfilesConfig struct {
	LoginProfiles map[string]LoginProfile `toml:"login_profiles"`
}

// LoginProfile mirrors the structure of a login profile.
type LoginProfile struct {
	Identifier     string `toml:"identifier"`
	CaptchaEnabled bool   `toml:"captcha_enabled"`
}

// CredentialsConfig mirrors the structure of credentials.toml
type CredentialsConfig struct {
	Credentials Credentials            `toml:"credentials"`
	URLMappings   map[string]string `toml:"url_mappings"`
}

// Credentials holds the user-specific login info.
type Credentials struct {
	URL            string `toml:"url"`
	Username       string `toml:"username"`
	Password       string `toml:"password"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
}

// LoadProfilesConfig reads and parses profiles.toml.
func LoadProfilesConfig(path string) (*ProfilesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ProfilesConfig
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// LoadCredentialsConfig reads and parses credentials.toml.
func LoadCredentialsConfig(path string) (*CredentialsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config CredentialsConfig
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, err
	}
	return &config, nil
}
