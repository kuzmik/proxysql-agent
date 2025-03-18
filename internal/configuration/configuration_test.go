package configuration

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// sample config file used in a couple of test functions.
const testConfigYAML = `---
start_delay: 30
log:
  level: "TRACE"
  format: "text"
run_mode: core
proxysql:
  address: "proxysql.vip:6032"
  username: "agent-user"
  password: "agent-password"
core:
  interval: 30
  podselector:
    namespace: test-namespace
    app: test-application
    component: test-component
satellite:
  interval: 60`

func TestValidations(t *testing.T) {
	os.Args = []string{"cmd"}

	pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

	viper.Reset()

	_, err := Configure()
	if err != nil {
		t.Fatalf("Configuration returned unexpected error: %v", err)
	}

	t.Run("validate run_mode", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			}
		}()

		viper.Reset()

		os.Args = []string{"cmd", "--run_mode=failure"}
		pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

		_, err := Configure()
		if err == nil {
			t.Error("expected error for invalid run_mode, got nil")
		}

		if err.Error() != "run_mode must be either 'core' or 'satellite'" {
			t.Errorf("expected error 'run_mode must be either 'core' or 'satellite'', got %v", err)
		}
	})

	t.Run("validate start_delay", func(t *testing.T) {
		viper.Reset()

		os.Args = []string{"cmd", "--start_delay=-1"}
		pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

		_, err := Configure()
		if err == nil {
			t.Error("expected error for negative start_delay, got nil")
		}

		if err.Error() != "start_delay cannot be < 0" {
			t.Errorf("expected error 'start_delay cannot be < 0', got %v", err)
		}
	})

	t.Run("validate core.interval", func(t *testing.T) {
		viper.Reset()

		os.Args = []string{"cmd", "--core.interval=-1"}
		pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

		_, err := Configure()
		if err == nil {
			t.Error("expected error for negative core.interval, got nil")
		}

		if err.Error() != "core.interval cannot be < 0" {
			t.Errorf("expected error 'core.interval cannot be < 0', got %v", err)
		}
	})

	t.Run("validate satellite.interval", func(t *testing.T) {
		viper.Reset()

		os.Args = []string{"cmd", "--satellite.interval=-1"}
		pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

		_, err := Configure()
		if err == nil {
			t.Error("expected error for negative satellite.interval, got nil")
		}

		if err.Error() != "satellite.interval cannot be < 0" {
			t.Errorf("expected error 'satellite.interval cannot be < 0', got %v", err)
		}
	})
}

func TestDefaults(t *testing.T) {
	os.Args = []string{"cmd"}
	pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

	viper.Reset()

	defaultsConfig, err := Configure()
	if err != nil {
		t.Fatalf("Configuration returned unexpected error: %v", err)
	}

	if got := defaultsConfig.Satellite.Interval; got != 10 {
		t.Errorf("default Satellite.Interval = %d, want %d", got, 10)
	}
}

func TestConfigFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	if _, err = tmpfile.Write([]byte(testConfigYAML)); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	tmpfile.Close()

	t.Setenv("AGENT_CONFIG_FILE", tmpfile.Name())

	os.Args = []string{"cmd"}
	pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

	fileConfig, err := Configure()
	if err != nil {
		t.Fatalf("Configure() returned unexpected error: %v", err)
	}

	tests := []struct {
		name   string
		got    interface{}
		want   interface{}
		errMsg string
	}{
		{"StartDelay", fileConfig.StartDelay, 30, "StartDelay"},
		{"Log.Level", fileConfig.Log.Level, "TRACE", "Log.Level"},
		{"RunMode", fileConfig.RunMode, "core", "RunMode"},
		{"ProxySQL.Address", fileConfig.ProxySQL.Address, "proxysql.vip:6032", "ProxySQL.Address"},
		{"ProxySQL.Username", fileConfig.ProxySQL.Username, "agent-user", "ProxySQL.Username"},
		{"ProxySQL.Password", fileConfig.ProxySQL.Password, "agent-password", "ProxySQL.Password"},
		{"Core.PodSelector.App", fileConfig.Core.PodSelector.App, "test-application", "Core.PodSelector.App"},
		{"Core.PodSelector.Component", fileConfig.Core.PodSelector.Component, "test-component", "Core.PodSelector.Component"},
		{"Satellite.Interval", fileConfig.Satellite.Interval, 60, "Satellite.Interval"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v = %v, want %v", tt.errMsg, tt.got, tt.want)
			}
		})
	}
}

func TestEnvironment(t *testing.T) {
	// Set up all environment variables
	envVars := map[string]string{
		"AGENT_START_DELAY":                "500",
		"AGENT_LOG_LEVEL":                  "env-WARN",
		"AGENT_LOG_FORMAT":                 "env-text",
		"AGENT_RUN_MODE":                   "satellite",
		"AGENT_PROXYSQL_ADDRESS":           "env-proxysql:6666",
		"AGENT_PROXYSQL_USERNAME":          "env-proxysql-user",
		"AGENT_PROXYSQL_PASSWORD":          "env-proxysql-password",
		"AGENT_CORE_PODSELECTOR_NAMESPACE": "env-proxysql-blue",
		"AGENT_CORE_PODSELECTOR_APP":       "env-proxysql-blue",
		"AGENT_CORE_PODSELECTOR_COMPONENT": "env-proxysql-core",
		"AGENT_SATELLITE_INTERVAL":         "60",
	}

	for k, v := range envVars {
		t.Setenv(k, v)
	}

	os.Args = []string{"cmd"}
	pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

	viper.Reset()

	envConfig, err := Configure()
	if err != nil {
		t.Fatalf("Configure() returned unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"StartDelay", envConfig.StartDelay, 500},
		{"Log.Level", envConfig.Log.Level, "env-WARN"},
		{"Log.Format", envConfig.Log.Format, "env-text"},
		{"RunMode", envConfig.RunMode, "satellite"},
		{"ProxySQL.Address", envConfig.ProxySQL.Address, "env-proxysql:6666"},
		{"ProxySQL.Username", envConfig.ProxySQL.Username, "env-proxysql-user"},
		{"ProxySQL.Password", envConfig.ProxySQL.Password, "env-proxysql-password"},
		{"Core.PodSelector.Namespace", envConfig.Core.PodSelector.Namespace, "env-proxysql-blue"},
		{"Core.PodSelector.App", envConfig.Core.PodSelector.App, "env-proxysql-blue"},
		{"Core.PodSelector.Component", envConfig.Core.PodSelector.Component, "env-proxysql-core"},
		{"Satellite.Interval", envConfig.Satellite.Interval, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestFlags(t *testing.T) {
	flags := []string{
		"cmd",
		"--start_delay=415",
		"--log.level=ERROR",
		"--log.format=text",
		"--run_mode=core",
		"--proxysql.address=86.75.30.9:9999",
		"--proxysql.username=nick",
		"--proxysql.password=NOWAY",
		"--core.interval=1000",
		"--core.podselector.app=proxysql-green",
		"--core.podselector.component=notcore",
		"--satellite.interval=5533",
	}
	os.Args = flags
	pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

	viper.Reset()

	flagConfig, err := Configure()
	if err != nil {
		t.Fatalf("Configure() returned unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"StartDelay", flagConfig.StartDelay, 415},
		{"Log.Level", flagConfig.Log.Level, "ERROR"},
		{"Log.Format", flagConfig.Log.Format, "text"},
		{"RunMode", flagConfig.RunMode, "core"},
		{"ProxySQL.Address", flagConfig.ProxySQL.Address, "86.75.30.9:9999"},
		{"ProxySQL.Username", flagConfig.ProxySQL.Username, "nick"},
		{"ProxySQL.Password", flagConfig.ProxySQL.Password, "NOWAY"},
		{"Core.Interval", flagConfig.Core.Interval, 1000},
		{"Core.PodSelector.App", flagConfig.Core.PodSelector.App, "proxysql-green"},
		{"Core.PodSelector.Component", flagConfig.Core.PodSelector.Component, "notcore"},
		{"Satellite.Interval", flagConfig.Satellite.Interval, 5533},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestPrecedence(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	if _, err = tmpfile.Write([]byte(testConfigYAML)); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	tmpfile.Close()

	t.Setenv("AGENT_CONFIG_FILE", tmpfile.Name())

	t.Run("env overwrites config file", func(t *testing.T) {
		viper.Reset()
		t.Setenv("AGENT_CORE_PODSELECTOR_COMPONENT", "env-test")

		os.Args = []string{"cmd"}
		pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

		configs, err := Configure()
		if err != nil {
			t.Fatalf("Configure() returned unexpected error: %v", err)
		}

		// Check value from config file remains unchanged
		if got := configs.StartDelay; got != 30 {
			t.Errorf("StartDelay = %v, want %v (from config file)", got, 30)
		}

		// Check value overwritten by environment variable
		if got := configs.Core.PodSelector.Component; got != "env-test" {
			t.Errorf("Core.PodSelector.Component = %v, want %v (from env)", got, "env-test")
		}
	})

	t.Run("flag overwrites config file and env", func(t *testing.T) {
		viper.Reset()
		t.Setenv("AGENT_CORE_PODSELECTOR_COMPONENT", "env-test")

		os.Args = []string{"cmd", "--core.podselector.component=flagtest"}
		pflag.CommandLine = pflag.NewFlagSet("cmd", pflag.ContinueOnError)

		configs, err := Configure()
		if err != nil {
			t.Fatalf("Configure() returned unexpected error: %v", err)
		}

		// Check value overwritten by flag takes precedence over both config file and env
		if got := configs.Core.PodSelector.Component; got != "flagtest" {
			t.Errorf("Core.PodSelector.Component = %v, want %v (from flag)", got, "flagtest")
		}
	})
}
