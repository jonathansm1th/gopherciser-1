package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/qlik-oss/gopherciser/buildmetrics"
	"github.com/qlik-oss/gopherciser/config"
	"github.com/qlik-oss/gopherciser/helpers"
	"github.com/qlik-oss/gopherciser/profile"
	"github.com/qlik-oss/gopherciser/scenario"
	"github.com/qlik-oss/gopherciser/senseobjdef"
	"github.com/spf13/cobra"
)

type (
	// JSONParseError error during unmarshal of JSON file
	JSONParseError string
	// JSONValidateError error during validation of JSON file
	JSONValidateError string
	// LogFormatError error resolving log format
	LogFormatError string
	// ObjectDefError error reading object definitions
	ObjectDefError string
	// ProfilingError error starting profiling
	ProfilingError string
	// MetricError error starting profiling
	MetricError string
	// OsError error when interacting with host OS
	OsError string
	// SummaryTypeError incorrect summary type
	SummaryTypeError string
)

var (
	traffic          bool
	trafficMetrics   bool
	debug            bool
	metricsPort      int
	metricsAddress   string
	metricsLabel     string
	metricsGroupings []string
	logFormat        string
	profTyp          string
	objDefFile       string
	summaryType      string
)

// Unfortunately go doesn't support negative exit codes,
// so same logic as sdkexerciser can't be used (positive for error count and negative for other errors)
// as exit codes seems to be limited to 8 bits (even though defined as an int),
// we will use setting the highest bit as a representation of negative values,
// i.e. errors not corresponding to simulation errors. Exit codes with highest bit set to 0
// corresponds to error count from simulation, where 0x7F will represent >127 errors.
const (
	// ExitCodeExecutionError error during execution
	ExitCodeExecutionError int = 0x80 + iota
	// ExitCodeJSONParseError error parsing JSON config
	ExitCodeJSONParseError
	// ExitCodeJSONValidateError validating JSON config failed
	ExitCodeJSONValidateError
	// ExitCodeLogFormatError error resolving log format
	ExitCodeLogFormatError
	// ExitCodeObjectDefError error reading object definitions
	ExitCodeObjectDefError
	// ExitCodeProfilingError error starting profiling
	ExitCodeProfilingError
	// ExitCodeMetricError error starting prometheus
	ExitCodeMetricError
	// ExitCodeOsError error when interacting with host OS
	ExitCodeOsError
	// ExitCodeSummaryTypeError incorrect summary type
	ExitCodeSummaryTypeError
)

// *** Custom errors ***

// Error implementation of Error interface
func (err JSONParseError) Error() string {
	return string(err)
}

// Error implementation of Error interface
func (err JSONValidateError) Error() string {
	return string(err)
}

// Error implementation of Error interface
func (err LogFormatError) Error() string {
	return string(err)
}

// Error implementation of Error interface
func (err ObjectDefError) Error() string {
	return string(err)
}

// Error implementation of Error interface
func (err ProfilingError) Error() string {
	return string(err)
}

// Error implementation of Error interface
func (err MetricError) Error() string {
	return string(err)
}

// Error implementation of Error interface
func (err OsError) Error() string {
	return string(err)
}

// Error incorrect summary type
func (err SummaryTypeError) Error() string {
	return string(err)
}

// *********************

// executeCmd represents the execute command
var executeCmd = &cobra.Command{
	Use:     "execute",
	Aliases: []string{"x"},
	Short:   "execute exerciser scenario towards a sense installation",
	Long:    `execute exerciser scenario towards a sense installation`,
	Run: func(cmd *cobra.Command, args []string) {
		if cfgFile == "" {
			_, _ = os.Stderr.WriteString("Error: No config provided\n")
			if err := cmd.Help(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
			}
			return
		}

		if execErr := execute(); execErr != nil {
			errMsg := "Unknown error"
			var exitCode int

			cause := errors.Cause(execErr)
			switch cause.(type) {
			case JSONParseError:
				errMsg = fmt.Sprint("JSONParseError: ", execErr)
				exitCode = ExitCodeJSONParseError
			case JSONValidateError:
				errMsg = fmt.Sprint("JSONValidateError: ", execErr)
				exitCode = ExitCodeJSONValidateError
			case LogFormatError:
				errMsg = fmt.Sprint("LogFormatError: ", execErr)
				exitCode = ExitCodeLogFormatError
			case ObjectDefError:
				errMsg = fmt.Sprint("ObjectDefError: ", execErr)
				exitCode = ExitCodeObjectDefError
			case ProfilingError:
				errMsg = fmt.Sprint("ProfilingError: ", execErr)
				exitCode = ExitCodeProfilingError
			case MetricError:
				errMsg = fmt.Sprint("MetricError: ", execErr)
				exitCode = ExitCodeMetricError
			case OsError:
				errMsg = fmt.Sprint("OsError: ", execErr)
				exitCode = ExitCodeOsError
			case SummaryTypeError:
				errMsg = fmt.Sprint("SummaryError: ", execErr)
				exitCode = ExitCodeSummaryTypeError
			case *multierror.Error:
				mErr := cause.(*multierror.Error)
				errCount := len(mErr.Errors)
				if mErr != nil && errCount > 0 {
					errMsg = fmt.Sprintf("%d errors occurred:\nFirst error: %s", errCount, mErr.Errors[0].Error())
				}
				if errCount > 0x7F {
					errCount = 0x7F
				}
				exitCode = errCount
			default:
				// only one error
				errMsg = fmt.Sprint("1 error occurred:\n", execErr)
				exitCode = 1
			}

			_, _ = fmt.Fprintf(os.Stderr, "%s\n", errMsg)
			os.Exit(exitCode)
		}
	},
}

func init() {
	RootCmd.AddCommand(executeCmd)
	AddAllSharedParameters(executeCmd)

	// Custom object definitions
	executeCmd.Flags().StringVarP(&objDefFile, "definitions", "d", "", `Custom object definitions and overrides.`)

	// Logging
	executeCmd.Flags().BoolVarP(&traffic, "traffic", "t", false, "Log traffic. Logging traffic is heavy and should only be done for debugging purposes.")
	executeCmd.Flags().BoolVarP(&trafficMetrics, "trafficmetrics", "m", false, "Log traffic metrics.")
	executeCmd.Flags().BoolVar(&debug, "debug", false, "Log debug info.")
	executeCmd.Flags().StringVar(&logFormat, "logformat", "", getLogFormatHelpString())
	executeCmd.Flags().StringVar(&summaryType, "summary", "", getSummaryTypeHelpString())

	// Prometheus
	executeCmd.Flags().IntVar(&metricsPort, "metrics", 0, "Export via http prometheus metrics.")
	executeCmd.Flags().StringVar(&metricsAddress, "metricsaddress", "", "If set other than empty string then Push otherwise pull, will be appended by port.")
	executeCmd.Flags().StringVar(&metricsLabel, "metricslabel", "gopherciser", "The job label to use for push metrics")
	executeCmd.Flags().StringSliceVarP(&metricsGroupings, "metricsgroupingkey", "g", nil, "The grouping keys (in key=value form) to use for push metrics. Specify multiple times for more grouping keys.")

	// profiling
	executeCmd.Flags().StringVar(&profTyp, "profile", "", profile.Help())
}

func execute() error {

	// === config section ===
	cfg, errUnmarshal := unmarshalConfigFile()
	if errUnmarshal != nil {
		return JSONParseError(errUnmarshal.Error())
	}

	if err := cfg.Validate(); err != nil {
		return JSONValidateError(err.Error())
	}

	// === logging section ===

	if trafficMetrics {
		cfg.SetTrafficMetricsLogging()
	}

	if traffic {
		cfg.SetTrafficLogging()
	}

	if debug {
		cfg.SetDebugLogging()
	}

	if logFormat != "" {
		var errLogformat error
		cfg.Settings.LogSettings.Format, errLogformat = resolveLogFormat(logFormat)
		if errLogformat != nil {
			return LogFormatError(fmt.Sprintf("error resolving log format<%s>: %v", logFormat, errLogformat))
		}
	}

	if summaryType != "" {
		if summary, errSummaryType := resolveSummaryType(); errSummaryType != nil {
			return SummaryTypeError(fmt.Sprintf("error resolving summary type<%s>: %v", summaryType, errSummaryType))
		} else {
			cfg.Settings.LogSettings.Summary = summary
		}
	}

	// === object definition section ===
	if err := ReadObjectDefinitions(); err != nil {
		return err
	}

	// === profiling section ===
	if profTyp != "" {
		defer func() {
			if err := profile.Close(); err != nil { //safe to use even if profiler was not started
				_, _ = fmt.Fprintf(os.Stderr, "error closiung profiler: %v", err)
			}
		}()

		typ, profErr := profile.ResolveParameter(profTyp)
		if profErr != nil {
			return ProfilingError(profErr.Error())
		}
		profErr = profile.StartProfiler(typ)
		if profErr != nil {
			return ProfilingError(fmt.Sprint("failed to start profiler. ", profErr))
		}
	}

	// === Handle SIGINT ===
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cancel()
	}()

	// === Prometheus section ===
	if metricsPort > 0 {
		// Check if push or pull by looking at whether address is set or not
		if metricsAddress == "" {
			//Prometheus metrics will be pulled from the endpoint /metrics
			err := buildmetrics.PullMetrics(ctx, metricsPort, scenario.RegisteredActions())
			if err != nil {
				return MetricError(fmt.Sprintf("failed to start prometheus : %s ", err))
			}
		} else {
			//Prometheus metrics will be pushed to a pushgateway
			err := buildmetrics.PushMetrics(ctx, metricsPort, metricsAddress, metricsLabel, metricsGroupings, scenario.RegisteredActions())
			if err != nil {
				return MetricError(fmt.Sprintf("failed to start prometheus : %s ", err))
			}
		}
	}

	// Data for variable templates
	templateData := struct {
		ConfigFile string
	}{strings.Split(filepath.Base(cfgFile), ".")[0]}

	// === start execution ===
	return cfg.Execute(ctx, templateData)
}

func ReadObjectDefinitions() error {
	if objDefFile != "" {
		if _, err := senseobjdef.OverrideFromFile(objDefFile); err != nil {
			return ObjectDefError(fmt.Sprintf("failed to read config from file<%s>): %v", objDefFile, err))
		}
	}

	return nil
}

func getLogFormatHelpString() string {
	buf := helpers.NewBuffer()
	buf.WriteString("Set a log format to be used. One of:\n")
	config.LogFormatType(0).GetEnumMap().ForEachSorted(func(k int, v string) {
		addEnumToBuf(buf, k, v)
	})
	buf.WriteString("Defaults to in-script definition and falls back on ")
	defaultFormat, _ := config.LogFormatType(0).GetEnumMap().String(0)
	buf.WriteString(defaultFormat)
	buf.WriteString("\n")
	return buf.String()
}

func addEnumToBuf(buf *helpers.Buffer, k int, v string) {
	buf.WriteString("[")
	buf.WriteString(strconv.Itoa(k))
	buf.WriteString("]: ")
	buf.WriteString(v)
	buf.WriteString("\n")
}

func getSummaryTypeHelpString() string {
	buf := helpers.NewBuffer()
	buf.WriteString("Set a summary type to be used. One of:\n")
	config.SummaryType(0).GetEnumMap().ForEachSorted(func(k int, v string) {
		addEnumToBuf(buf, k, v)
	})
	return buf.String()
}

func resolveLogFormat(param string) (config.LogFormatType, error) {
	var i int
	var err error

	// Do we have an int?
	if i, err = strconv.Atoi(param); err != nil {
		// it's a string
		i, err = config.LogFormatType(0).GetEnumMap().Int(param)
		if err != nil {
			return -1, errors.Wrapf(err, "failed to parse <%s> to log format", param)
		}
	}
	// it's an int

	// make sure it's a valid type
	_, err = config.LogFormatType(0).GetEnumMap().String(i)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to parse <%s> to log format", param)
	}

	return config.LogFormatType(i), nil
}

func resolveSummaryType() (config.SummaryType, error) {
	if i, err := strconv.Atoi(summaryType); err != nil {
		// it's a string
		i, err = config.SummaryType(0).GetEnumMap().Int(summaryType)
		if err != nil {
			return config.SummaryTypeDefault, errors.Errorf("Summary type<%s> does not exist", summaryType)
		}
		return config.SummaryType(i), nil
	} else {
		// it's an int
		_, err = config.SummaryType(0).GetEnumMap().String(i)
		if err != nil {
			return config.SummaryTypeDefault, errors.Errorf("Summary type<%s> does not exist", summaryType)
		}
		return config.SummaryType(i), nil
	}
}
