package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	MetricsConfigTypeCount = "count"
	MetricsConfigTypeGauge = "gauge"
)

func NewPluginContext(plugin unsafe.Pointer) (*PluginContext, error) {
	config, err := initConfig(plugin)
	if err != nil {
		return nil, err
	}
	client, err := initClient(plugin)
	if err != nil {
		return nil, err
	}
	logger, isDebug, err := initLogger(plugin)
	if err != nil {
		return nil, err
	}
	log.With(logger, "plugin", pluginName, "metric", config.Name)
	pCtx := PluginContext{
		MetricsConfig: *config,
		Client:        client,
		Logger:        logger,
		isDebug:       isDebug,
	}
	pCtx.Info("msg", "init", "config", *config)
	return &pCtx, nil
}

type PluginContext struct {
	MetricsConfig
	Client  *statsd.Client
	Logger  log.Logger
	isDebug bool
}

type MetricsConfig struct {
	CountConfig
	GaugeConfig
	Type        string
	Name        string
	Namespace   string
	SampleRate  float64
	StaticTags  map[string]string
	DynamicTags []string
}

type CountConfig struct {
	Method string
}

type GaugeConfig struct {
	ValueField string
}

func (c *PluginContext) send(record map[string]interface{}) (err error) {
	tags := c.getTags(record)
	switch c.Type {
	case MetricsConfigTypeCount:
		c.Debug("msg", "send called", "type", c.Type, "name", c.Name, "tags", tags, "rate", c.SampleRate)
		return c.Client.Incr(c.Name, tags, c.SampleRate)
	case MetricsConfigTypeGauge:
		value := float64(0)
		if c.ValueField != "" {
			value, err = strconv.ParseFloat(fmt.Sprintf("%s", record[c.ValueField]), 64)
			if err != nil {
				return
			}
		}
		c.Debug("msg", "send called", "type", c.Type, "name", c.Name, "value", value, "tags", tags, "rate", c.SampleRate)
		return c.Client.Gauge(c.Name, value, tags, c.SampleRate)
	default:
		return fmt.Errorf("unsupported metric type %s", c.Type)
	}
}

func (c *PluginContext) getTags(record map[string]interface{}) (tags []string) {
	for _, dTag := range c.DynamicTags {
		if v, ok := record[dTag]; ok {
			switch t := v.(type) {
			case string:
				tags = append(tags, fmt.Sprintf("%s:%s", dTag, t))
			default:
				c.Warn("msg", "dynamic tag is not a string", "tag", dTag)
			}
		}
	}
	for k, v := range c.StaticTags {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}
	return
}

func (c *PluginContext) log(levelLogger log.Logger, keyVals []interface{}) {
	for i := range keyVals {
		keyVals[i] = fmt.Sprintf("%v", keyVals[i])
	}
	levelLogger.Log(keyVals...)
}

func (c *PluginContext) Debug(keyVals ...interface{}) {
	if c.isDebug {
		c.log(level.Debug(c.Logger), keyVals)
	}
}

func (c *PluginContext) Info(keyVals ...interface{}) {
	c.log(level.Info(c.Logger), keyVals)
}

func (c *PluginContext) Warn(keyVals ...interface{}) {
	c.log(level.Warn(c.Logger), keyVals)
}

func (c *PluginContext) Error(keyVals ...interface{}) {
	c.log(level.Error(c.Logger), keyVals)
}

func initLogger(plugin unsafe.Pointer) (log.Logger, bool, error) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	logLevel := stringConf(plugin, "loglevel", "warn")
	switch logLevel {
	case "error":
		return level.NewFilter(logger, level.AllowError()), false, nil
	case "info":
		return level.NewFilter(logger, level.AllowInfo()), false, nil
	case "warn":
		return level.NewFilter(logger, level.AllowWarn()), false, nil
	case "debug":
		return level.NewFilter(logger, level.AllowDebug()), true, nil
	default:
		return nil, false, fmt.Errorf("log level %s is not supported", logLevel)
	}
}

func initClient(plugin unsafe.Pointer) (*statsd.Client, error) {
	url := stringConf(plugin, "url", "127.0.0.1:8125")
	ns := stringConf(plugin, "namespace", "")
	return statsd.New(url, statsd.WithNamespace(ns))
}

func initConfig(plugin unsafe.Pointer) (*MetricsConfig, error) {
	metricType := stringConf(plugin, "metric_type", "")
	if metricType == "" {
		return nil, fmt.Errorf("metric_type is required")
	}
	metricName := stringConf(plugin, "metric_name", "")
	if metricName == "" {
		return nil, fmt.Errorf("metric_name is required")
	}
	sampleRate := float64(1)
	rateStr := stringConf(plugin, "sample_rate", "")
	if rateStr != "" {
		var err error
		sampleRate, err = strconv.ParseFloat(rateStr, 64)
		if err != nil {
			return nil, err
		}
	}
	staticTags, err := mapConf(plugin, "metric_static_tags", nil)
	if err != nil {
		return nil, err
	}
	dynamicTags := sliceConf(plugin, "metric_dynamic_tags", nil)

	config := MetricsConfig{
		Type:        metricType,
		Name:        metricName,
		SampleRate:  sampleRate,
		StaticTags:  staticTags,
		DynamicTags: dynamicTags,
	}

	switch metricType {
	case MetricsConfigTypeCount:
	case MetricsConfigTypeGauge:
		valueField := stringConf(plugin, "value_field", "")
		if valueField == "" {
			return nil, fmt.Errorf("value_field is required for gauge metric")
		}
		config.ValueField = valueField
	default:
		return nil, fmt.Errorf("unknown metric type %s", metricType)
	}

	return &config, nil
}

func stringConf(plugin unsafe.Pointer, key string, def string) string {
	value := output.FLBPluginConfigKey(plugin, key)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	if value == "" {
		return strings.ToLower(def)
	}
	return strings.ToLower(value)
}

func sliceConf(plugin unsafe.Pointer, key string, def []string) []string {
	value := output.FLBPluginConfigKey(plugin, key)
	value = strings.TrimSpace(value)
	if value == "" {
		return def
	}
	result := strings.Split(value, ",")
	for i := range result {
		result[i] = strings.TrimSpace(result[i])
		result[i] = strings.Trim(result[i], "\"")
	}
	return result
}

func mapConf(plugin unsafe.Pointer, key string, def map[string]string) (map[string]string, error) {
	value := output.FLBPluginConfigKey(plugin, key)
	value = strings.TrimSpace(value)
	if value == "" {
		return def, nil
	}
	var j map[string]interface{}
	if err := json.Unmarshal([]byte(value), &j); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range j {
		result[k] = fmt.Sprintf("%s", v)
	}
	return result, nil
}

// toStringSlice: Code borrowed from Loki
// prevent base64-encoding []byte values (default json.Encoder rule) by
// converting them to strings
func toStringSlice(slice []interface{}) []interface{} {
	var s []interface{}
	for _, v := range slice {
		switch t := v.(type) {
		case []byte:
			s = append(s, string(t))
		case map[interface{}]interface{}:
			s = append(s, toStringMap(t))
		case []interface{}:
			s = append(s, toStringSlice(t))
		default:
			s = append(s, t)
		}
	}
	return s
}

// toStringMap: Code borrowed from Loki
func toStringMap(record map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range record {
		key, ok := k.(string)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case []byte:
			m[key] = string(t)
		case map[interface{}]interface{}:
			m[key] = toStringMap(t)
		case []interface{}:
			m[key] = toStringSlice(t)
		default:
			m[key] = v
		}
	}

	return m
}
