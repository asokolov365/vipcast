package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/stretchr/testify/require"
)

func TestLogger_SetupBasic(t *testing.T) {
	cfg := Config{Level: "INFO"}

	logger, err := Setup(&cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, logger)
}

func TestLogger_SetupInvalidLogLevel(t *testing.T) {
	cases := []struct {
		desc string
		cfg  Config
	}{
		{
			desc: "no log level",
			cfg:  Config{},
		},
		{
			desc: "empty log level",
			cfg:  Config{Level: ""},
		},
		{
			desc: "invalid log level",
			cfg:  Config{Level: "foobar"},
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			_, err := Setup(&c.cfg, nil)
			require.ErrorContains(t, err, "invalid log level:")
		})
	}
}

func TestLogger_SetupInvalidTimezone(t *testing.T) {
	cfg := Config{Level: "INFO", Timezone: "foobar"}

	_, err := Setup(&cfg, nil)
	require.ErrorContains(t, err, "cannot load timezone")
}

func TestLogger_SetupLoggerErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level: "ERROR",
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Error("test error msg")
	logger.Info("test info msg")

	output := buf.String()

	require.Contains(t, output, "[ERROR] test error msg")
	require.NotContains(t, output, "[INFO]  test info msg")
}

func TestLogger_SetupLoggerDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{Level: "DEBUG"}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Info("test info msg")
	logger.Debug("test debug msg")

	output := buf.String()

	require.Contains(t, output, "[INFO]  test info msg")
	require.Contains(t, output, "[DEBUG] test debug msg")
}

func TestLogger_SetupLoggerWithName(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Name:  "test-system",
		Level: "DEBUG",
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Warn("test warn msg")

	require.Contains(t, buf.String(), "[WARN]  test-system: test warn msg")
}

func TestLogger_SetupLoggerWithJSON(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Name:            "test-system",
		Level:           "DEBUG",
		LogJSON:         true,
		IncludeLocation: true,
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Warn("test warn msg")
	// fmt.Println(buf.String())
	var jsonOutput map[string]string
	err = json.Unmarshal(buf.Bytes(), &jsonOutput)
	require.NoError(t, err)
	require.Contains(t, jsonOutput, "@level")
	require.Equal(t, jsonOutput["@level"], "warn")
	require.Contains(t, jsonOutput, "@message")
	require.Equal(t, jsonOutput["@message"], "test warn msg")
}

func TestLogger_SetupLoggerNoTimestamp(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:             "INFO",
		DisableTimestamps: true,
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Info("test info msg")
	require.Equal(t, "[INFO]  test info msg\n", buf.String())
}

func TestLogger_SetupLoggerIncludeLocation(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Name:            "test",
		Level:           "INFO",
		IncludeLocation: true,
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	_, _, line, _ := runtime.Caller(0)

	logger.Warn("this is test", "who", "programmer", "why", "testing is fun")

	str := buf.String()
	dataIdx := strings.IndexByte(str, ' ')
	// Strip timestamp
	rest := str[dataIdx+1:]
	expected := fmt.Sprintf(
		"[WARN]  logging/logger_test.go:%d: test: this is test: who=programmer why=\"testing is fun\"\n",
		line+2,
	)
	require.Equal(t, expected, rest)
}

func TestLogger_LoggerIncrementsMsgCounterMetric(t *testing.T) {
	var buf bytes.Buffer
	var cnt *metrics.Counter
	var metricName string
	var metricValue uint64 = 0

	cfg := Config{
		Name:              "test",
		Level:             "INFO",
		DisableTimestamps: true,
		IncludeLocation:   true,
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	_, _, line, _ := runtime.Caller(0)

	logger.Warn("this is test", "who", "programmer", "why", "testing is fun")
	str := buf.String()
	expected := fmt.Sprintf(
		"[WARN]  logging/logger_test.go:%d: test: this is test: who=programmer why=\"testing is fun\"\n",
		line+2,
	)
	require.Equal(t, expected, str)

	metricNames := metrics.ListMetricNames()
	for _, metricName = range metricNames {
		if strings.HasPrefix(metricName, LogMessagesCounterMetricName) &&
			strings.Contains(metricName, fmt.Sprintf("logging/logger_test.go:%d", line+2)) {
			cnt = metrics.GetOrCreateCounter(metricName)
			metricValue = cnt.Get()
			// fmt.Printf("%s=%d\n", metricName, cnt.Get())
			break
		}
	}
	require.GreaterOrEqual(t, uint64(1), metricValue)
	require.Contains(t, metricName, fmt.Sprintf("level=%q", "warn"))
}

// func TestLogger_LoggerMessageLengthLimit(t *testing.T) {
// 	var buf bytes.Buffer
// 	cfg := Config{
// 		Level:             "INFO",
// 		DisableTimestamps: true,
// 		MaxMessageLength:  50,
// 	}

// 	logger, err := Setup(&cfg, &buf)
// 	require.NoError(t, err)
// 	require.NotNil(t, logger)

// 	logger.Info("here we are going to test a very loooooooong message")
// 	require.Equal(t, "[INFO]  here we are goin..ery loooooooong message\n", buf.String())
// }

// func TestLogger_LoggerJsonMessageLengthLimit(t *testing.T) {
// 	var buf bytes.Buffer
// 	cfg := Config{
// 		Name:             "test-system",
// 		Level:            "DEBUG",
// 		LogJSON:          true,
// 		IncludeLocation:  true,
// 		MaxMessageLength: 50,
// 	}

// 	logger, err := Setup(&cfg, &buf)
// 	require.NoError(t, err)
// 	require.NotNil(t, logger)

// 	logger.Warn("here we are going to test a very loooooooong message")

// 	var jsonOutput map[string]string
// 	err = json.Unmarshal(buf.Bytes(), &jsonOutput)
// 	require.NoError(t, err)
// 	require.Contains(t, jsonOutput, "@level")
// 	require.Equal(t, "warn", jsonOutput["@level"])
// 	require.Contains(t, jsonOutput, "@message")
// 	require.Equal(t, "here we are going to tes..very loooooooong message", jsonOutput["@message"])
// 	require.Contains(t, jsonOutput, "@location")
// 	require.Contains(t, jsonOutput["@location"], "jetdragon/logging/logger_test.go:")
// }

func TestLogger_LoggerRateLimit(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Name:                 "test-rate-limit",
		Level:                "INFO",
		DisableTimestamps:    true,
		WarnsPerSecondLimit:  5,
		ErrorsPerSecondLimit: 10,
	}

	logger, err := Setup(&cfg, &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)

	var line int
	for i := 0; i < 5; i++ {
		for j := 0; j < 100; j++ {
			_, _, line, _ = runtime.Caller(0)
			logger.Warn("test warn msg", "bulk", i+1, "attempt", j+1)
			logger.Error("test error msg", "bulk", i+1, "attempt", j+1)
		}
		msgsNum := len(strings.Split(buf.String(), "\n")) - 1 // -1 trims empty line
		require.Equal(t, 15, msgsNum)                         // 5 warn + 10 error
		buf.Reset()
		time.Sleep(time.Second)
	}

	var metricWarnsValue uint64
	var metricErrorsValue uint64
	metricNames := metrics.ListMetricNames()
	for _, metricName := range metricNames {
		fmt.Println(metricName)
		if strings.HasPrefix(metricName, LogMessagesCounterMetricName) &&
			strings.Contains(metricName, fmt.Sprintf("logging/logger_test.go:%d", line+1)) {
			cnt := metrics.GetOrCreateCounter(metricName)
			metricWarnsValue = cnt.Get()
		}
		if strings.HasPrefix(metricName, LogMessagesCounterMetricName) &&
			strings.Contains(metricName, fmt.Sprintf("logging/logger_test.go:%d", line+2)) {
			cnt := metrics.GetOrCreateCounter(metricName)
			metricErrorsValue = cnt.Get()
		}
	}
	// Check vipcast_log_messages_total has been updated correctly
	require.Equal(t, uint64(500), metricWarnsValue)
	require.Equal(t, uint64(500), metricErrorsValue)
}
