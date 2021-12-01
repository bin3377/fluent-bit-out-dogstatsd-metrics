package main

import "C"

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
)

const pluginName = "dogstatsd_metrics"

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	// fmt.Printf("[%s] FLBPluginRegister called\n", pluginName)
	return output.FLBPluginRegister(
		def,
		pluginName,
		"Exporting dogstatsd metrics.",
	)
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	// fmt.Printf("[%s] FLBPluginInit called\n", pluginName)
	ctx, err := NewPluginContext(plugin)
	if err != nil {
		panic(err)
	}

	// Set the context to point to any Go variable
	output.FLBPluginSetContext(plugin, ctx)
	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	// fmt.Printf("[%s] FLBPluginFlush called\n", pluginName)
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	// fmt.Printf("[%s] FLBPluginFlushCtx called\n", pluginName)
	// Type assert context back into the original type for the Go variable
	pCtx := output.FLBPluginGetContext(ctx).(*PluginContext)
	dec := output.NewDecoder(data, int(length))

	count := 0
	for {
		ret, ts, rec := output.GetRecord(dec)
		if ret != 0 {
			break
		}
		var timestamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timestamp = ts.(output.FLBTime).Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			pCtx.Warn("msg", "time provided invalid, defaulting to now.")
			timestamp = time.Now()
		}

		record := toStringMap(rec)
		if err := pCtx.send(record); err != nil {
			msg := fmt.Sprintf("[%d] %v: [%s] {", count, C.GoString(tag), timestamp.String())
			for k, v := range record {
				msg += fmt.Sprintf("\"%s\": %s,", k, v)
			}
			msg += fmt.Sprintf("} - %v", err)
			pCtx.Error("msg", "send failure", "err", err, "tag", C.GoString(tag), "timestamp", timestamp, "record", record)
		} else {
			pCtx.Debug("msg", "FLBPluginFlushCtx", "tag", C.GoString(tag), "timestamp", timestamp, "record", record)
		}
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}
