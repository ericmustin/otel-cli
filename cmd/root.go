package cmd

import (
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config stores the runtime configuration for otel-cli.
// This is used as a singleton as "config" and accessed from many other files.
// Data structure is public so that it can serialize to json easily.
type Config struct {
	Endpoint    string            `json:"endpoint"`
	Timeout     string            `json:"timeout"`
	Headers     map[string]string `json:"headers"` // TODO: needs json marshaler hook to mask tokens
	Insecure    bool              `json:"insecure"`
	Blocking    bool              `json:"blocking"`
	NoTlsVerify bool              `json:"no_tls_verify"`

	ServiceName string            `json:"service_name"`
	SpanName    string            `json:"span_name"`
	Kind        string            `json:"span_kind"`
	Attributes  map[string]string `json:"span_attributes"`

	TraceparentCarrierFile string `json:"traceparent_carrier_file"`
	TraceparentIgnoreEnv   bool   `json:"traceparent_ignore_env"`
	TraceparentPrint       bool   `json:"traceparent_print"`
	TraceparentPrintExport bool   `json:"traceparent_print_export"`
	TraceparentRequired    bool   `json:"traceparent_required"`

	BackgroundParentPollMs int    `json:"background_parent_poll_ms"`
	BackgroundSockdir      string `json:"background_socket_directory"`
	BackgroundWait         bool   `json:"background_wait"`

	SpanStartTime string `json:"span_start_time"`
	SpanEndTime   string `json:"span_end_time"`
	EventName     string `json:"event_name"`
	EventTime     string `json:"event_time"`

	CfgFile string `json:"config_file"`
}

const defaultOtlpEndpoint = "localhost:4317"
const spanBgSockfilename = "otel-cli-background.sock"

var exitCode int
var config Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "otel-cli",
	Short: "CLI for creating and sending OpenTelemetry spans and events.",
	Long:  `A command-line interface for generating OpenTelemetry data on the command line.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initViperConfig)
	cobra.EnableCommandSorting = false
	rootCmd.Flags().SortFlags = false
}

// addCommonParams adds the --config and --endpoint params to the command.
func addCommonParams(cmd *cobra.Command) {
	// --config / -c a viper configuration file
	cmd.Flags().StringVarP(&config.CfgFile, "config", "c", "", "config file (default is $HOME/.otel-cli.yaml)")
	// --endpoint an endpoint to send otlp output to
	cmd.Flags().StringVar(&config.Endpoint, "endpoint", "", "dial address for the desired OTLP/gRPC endpoint")
	// --timeout a default timeout to use in all otel-cli operations (default 1s)
	cmd.Flags().StringVar(&config.Timeout, "timeout", "1s", "timeout for otel-cli operations, all timeouts in otel-cli use this value")

	var common_env_flags = map[string]string{
		"endpoint": "OTEL_EXPORTER_OTLP_ENDPOINT",
		"timeout":  "OTEL_EXPORTER_OTLP_TIMEOUT",
	}

	for config_key, env_value := range common_env_flags {
		viper.BindPFlag(config_key, cmd.Flags().Lookup(config_key))
		viper.BindEnv(config_key, env_value)
	}
}

// addClientParams adds the common CLI flags for e.g. span and exec to the command.
// envvars are named according to the otel specs, others use the OTEL_CLI prefix
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/sdk-environment-variables.md
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
func addClientParams(cmd *cobra.Command) {
	config.Headers = make(map[string]string)
	// OTEL_EXPORTER standard env and variable params
	cmd.Flags().BoolVar(&config.Insecure, "insecure", false, "refuse to connect if TLS is unavailable (true by default when endpoint is localhost)")
	cmd.Flags().StringToStringVar(&config.Headers, "otlp-headers", map[string]string{}, "a comma-sparated list of key=value headers to send on OTLP connection")
	cmd.Flags().BoolVar(&config.Blocking, "otlp-blocking", false, "block on connecting to the OTLP server before proceeding")

	// OTEL_CLI trace propagation options
	cmd.Flags().BoolVar(&config.TraceparentRequired, "tp-required", false, "when set to true, fail and log if a traceparent can't be picked up from TRACEPARENT ennvar or a carrier file")
	cmd.Flags().StringVar(&config.TraceparentCarrierFile, "tp-carrier", "", "a file for reading and WRITING traceparent across invocations")
	cmd.Flags().BoolVar(&config.TraceparentIgnoreEnv, "tp-ignore-env", false, "ignore the TRACEPARENT envvar even if it's set")
	cmd.Flags().BoolVar(&config.TraceparentPrint, "tp-print", false, "print the trace id, span id, and the w3c-formatted traceparent representation of the new span")
	cmd.Flags().BoolVarP(&config.TraceparentPrintExport, "tp-export", "p", false, "same as --tp-print but it puts an 'export ' in front so it's more convinenient to source in scripts")
	cmd.Flags().BoolVar(&config.NoTlsVerify, "no-tls-verify", false, "enable it when TLS is enabled and you want to ignore the certificate validation. This is common when you are testing and usign self-signed certificates.")

	var client_env_flags = map[string]string{
		"insecure":      "OTEL_EXPORTER_OTLP_INSECURE",
		"otlp-headers":  "OTEL_EXPORTER_OTLP_HEADERS",
		"otlp-blocking": "OTEL_EXPORTER_OTLP_BLOCKING",
		"tp-required":   "OTEL_CLI_TRACEPARENT_REQUIRED",
		"tp-carrier":    "OTEL_CLI_CARRIER_FILE",
		"tp-ignore-env": "OTEL_CLI_IGNORE_ENV",
		"tp-print":      "OTEL_CLI_PRINT_TRACEPARENT",
		"tp-export":     "OTEL_CLI_EXPORT_TRACEPARENT",
		"no-tls-verify": "OTEL_CLI_NO_TLS_VERIFY",
	}
	for config_key, env_value := range client_env_flags {
		viper.BindPFlag(config_key, cmd.Flags().Lookup(config_key))
		viper.BindEnv(config_key, env_value)
	}
}

func addSpanParams(cmd *cobra.Command) {
	// --name / -s
	cmd.Flags().StringVarP(&config.SpanName, "name", "s", "todo-generate-default-span-names", "set the name of the span")
	// --service / -n
	cmd.Flags().StringVarP(&config.ServiceName, "service", "n", "otel-cli", "set the name of the application sent on the traces")
	// --kind / -k
	cmd.Flags().StringVarP(&config.Kind, "kind", "k", "client", "set the trace kind, e.g. internal, server, client, producer, consumer")
	var span_env_flags = map[string]string{
		"service": "OTEL_CLI_SERVICE_NAME",
		"kind":    "OTEL_CLI_TRACE_KIND",
	}
	for config_key, env_value := range span_env_flags {
		viper.BindPFlag(config_key, cmd.Flags().Lookup(config_key))
		viper.BindEnv(config_key, env_value)
	}
}

func addAttrParams(cmd *cobra.Command) {
	// --attrs key=value,foo=bar
	config.Attributes = make(map[string]string)
	cmd.Flags().StringToStringVarP(&config.Attributes, "attrs", "a", map[string]string{}, "a comma-separated list of key=value attributes")
	viper.BindPFlag("attrs", cmd.Flags().Lookup("attrs"))
	viper.BindEnv("attrs", "OTEL_CLI_ATTRIBUTES")
}

func initViperConfig() {
	if config.CfgFile != "" {
		viper.SetConfigFile(config.CfgFile)
	} else {
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigName(".otel-cli") // e.g. ~/.otel-cli.yaml
	}

	if err := viper.ReadInConfig(); err != nil {
		// We want to suppress errors here if the config is not found, but only if the user has not expressly given us a location to search.
		// Otherwise, we'll raise any config-reading error up to the user.
		_, cfgNotFound := err.(viper.ConfigFileNotFoundError)
		if config.CfgFile != "" || !cfgNotFound {
			cobra.CheckErr(err)
		}
	}
	viper.Unmarshal(&config)
}
