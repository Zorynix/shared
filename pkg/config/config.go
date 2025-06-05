package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

var Validator = validator.New()

type BaseConfig struct {
	Environment string `yaml:"environment" validate:"required,oneof=development staging production"`
	Debug       bool   `yaml:"debug"`
	LogLevel    string `yaml:"log_level" validate:"required,oneof=debug info warn error fatal"`
}

type DatabaseConfig struct {
	Host            string `yaml:"host" validate:"required"`
	Port            int    `yaml:"port" validate:"required,min=1,max=65535"`
	Database        string `yaml:"database" validate:"required"`
	Username        string `yaml:"username" validate:"required"`
	Password        string `yaml:"password"`
	DSN             string `yaml:"dsn"`
	SSLMode         string `yaml:"ssl_mode" validate:"oneof=disable require verify-ca verify-full"`
	MaxOpenConns    int    `yaml:"max_open_conns" validate:"min=1"`
	MaxIdleConns    int    `yaml:"max_idle_conns" validate:"min=1"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
}

type RedisConfig struct {
	Addr         string `yaml:"addr" validate:"required"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db" validate:"min=0"`
	PoolSize     int    `yaml:"pool_size" validate:"min=1"`
	MinIdleConns int    `yaml:"min_idle_conns" validate:"min=0"`
}

type GRPCConfig struct {
	Port    int `yaml:"port" validate:"required,min=1,max=65535"`
	Timeout int `yaml:"timeout" validate:"min=1"`
}

type SecurityConfig struct {
	JWTSecret              string `yaml:"jwt_secret" validate:"required,min=32"`
	JWTExpiration          string `yaml:"jwt_expiration" validate:"required"`
	RefreshExpiration      string `yaml:"refresh_expiration" validate:"required"`
	RateLimitRPS           int    `yaml:"rate_limit_rps" validate:"min=1"`
	RateLimitBurst         int    `yaml:"rate_limit_burst" validate:"min=1"`
	EnableStrictMode       bool   `yaml:"enable_strict_mode"`
	PasswordMinLength      int    `yaml:"password_min_length" validate:"min=6"`
	PasswordRequireUpper   bool   `yaml:"password_require_upper"`
	PasswordRequireLower   bool   `yaml:"password_require_lower"`
	PasswordRequireDigit   bool   `yaml:"password_require_digit"`
	PasswordRequireSpecial bool   `yaml:"password_require_special"`
}

type MonitoringConfig struct {
	Enabled        bool   `yaml:"enabled"`
	PrometheusPort int    `yaml:"prometheus_port" validate:"min=1,max=65535"`
	MetricsPath    string `yaml:"metrics_path"`
	HealthPath     string `yaml:"health_path"`
}

func LoadConfig[T any](configPath string, target *T) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if err := applyEnvironmentOverrides(target); err != nil {
		return fmt.Errorf("failed to apply environment overrides: %w", err)
	}

	if err := Validator.Struct(target); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}

func LoadConfigWithEnvironment[T any](basePath string, environment string, target *T) error {
	envConfigPath := strings.Replace(basePath, ".yaml", fmt.Sprintf(".%s.yaml", environment), 1)

	configPath := basePath
	if _, err := os.Stat(envConfigPath); err == nil {
		configPath = envConfigPath
	}

	return LoadConfig(configPath, target)
}

func applyEnvironmentOverrides(config interface{}) error {
	v := reflect.ValueOf(config)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config must be a pointer to struct")
	}

	return applyEnvOverridesToStruct(v.Elem(), "")
}

func applyEnvOverridesToStruct(v reflect.Value, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		if !field.CanSet() {
			continue
		}

		yamlTag := fieldType.Tag.Get("yaml")
		if yamlTag == "" || yamlTag == "-" {
			continue
		}

		yamlName := strings.Split(yamlTag, ",")[0]

		envName := buildEnvName(prefix, yamlName)

		if envValue := os.Getenv(envName); envValue != "" {
			if err := setFieldFromString(field, envValue); err != nil {
				return fmt.Errorf("failed to set field %s from env %s: %w", fieldType.Name, envName, err)
			}
		}

		if field.Kind() == reflect.Struct {
			if err := applyEnvOverridesToStruct(field, envName); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildEnvName(prefix, name string) string {
	envName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if prefix != "" {
		return prefix + "_" + envName
	}
	return envName
}

func setFieldFromString(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		if value == "true" || value == "1" {
			field.SetBool(true)
		} else if value == "false" || value == "0" {
			field.SetBool(false)
		} else {
			return fmt.Errorf("invalid boolean value: %s", value)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if intVal, err := parseInt64(value); err == nil {
			field.SetInt(intVal)
		} else {
			return err
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if uintVal, err := parseUint64(value); err == nil {
			field.SetUint(uintVal)
		} else {
			return err
		}
	case reflect.Float32, reflect.Float64:
		if floatVal, err := parseFloat64(value); err == nil {
			field.SetFloat(floatVal)
		} else {
			return err
		}
	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}
	return nil
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseUint64(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

func parseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func ValidateConfig(config interface{}) error {
	return Validator.Struct(config)
}

func GetEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = os.Getenv("ENV")
	}
	if env == "" {
		return "development"
	}
	return env
}
