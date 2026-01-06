package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the policy enforcer
type Config struct {
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	EFlint   EFlintConfig   `mapstructure:"eflint"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

// RabbitMQConfig holds RabbitMQ connection settings
type RabbitMQConfig struct {
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	Username       string        `mapstructure:"username"`
	Password       string        `mapstructure:"password"`
	Queue          string        `mapstructure:"queue"`
	Exchange       string        `mapstructure:"exchange"`
	RoutingKey     string        `mapstructure:"routing_key"`
	PrefetchCount  int           `mapstructure:"prefetch_count"`
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay"`
}

// EFlintConfig holds eFLINT server settings
type EFlintConfig struct {
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	ServerPath     string        `mapstructure:"server_path"`
	ModelPath      string        `mapstructure:"model_path"`
	Timeout        time.Duration `mapstructure:"timeout"`
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay"`
	MaxRetries     int           `mapstructure:"max_retries"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level       string `mapstructure:"level"`
	Format      string `mapstructure:"format"`
	Output      string `mapstructure:"output"`
	Development bool   `mapstructure:"development"`
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set config file location
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// Read environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("PE") // Policy Enforcer

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
