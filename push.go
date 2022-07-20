package metrics

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// InitPushProcessMetrics sets up periodic push for 'process_*' metrics to the given pushURL with the given interval.
//
// extraLabels may contain comma-separated list of `label="value"` labels, which will be added
// to all the metrics before pushing them to pushURL.
//
// The metrics are pushed to pushURL in Prometheus text exposition format.
// See https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md#text-based-format
//
// It is recommended pushing metrics to /api/v1/import/prometheus endpoint according to
// https://docs.victoriametrics.com/#how-to-import-data-in-prometheus-exposition-format
//
// It is OK calling InitPushProcessMetrics multiple times with different pushURL -
// in this case metrics are pushed to all the provided pushURL urls.
func InitPushProcessMetrics(pushURL string, interval time.Duration, extraLabels string) {
	writeMetrics := func(w io.Writer) {
		WriteProcessMetrics(w)
	}
	initPush(pushURL, interval, extraLabels, writeMetrics)
}

// InitPush sets up periodic push for globally registered metrics to the given pushURL with the given interval.
//
// extraLabels may contain comma-separated list of `label="value"` labels, which will be added
// to all the metrics before pushing them to pushURL.
//
// If pushProcessMetrics is set to true, then 'process_*' metrics are also pushed to pushURL.
//
// The metrics are pushed to pushURL in Prometheus text exposition format.
// See https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md#text-based-format
//
// It is recommended pushing metrics to /api/v1/import/prometheus endpoint according to
// https://docs.victoriametrics.com/#how-to-import-data-in-prometheus-exposition-format
//
// It is OK calling InitPush multiple times with different pushURL -
// in this case metrics are pushed to all the provided pushURL urls.
func InitPush(pushURL string, interval time.Duration, extraLabels string, pushProcessMetrics bool) {
	writeMetrics := func(w io.Writer) {
		defaultSet.WritePrometheus(w)
		if pushProcessMetrics {
			WriteProcessMetrics(w)
		}
	}
	initPush(pushURL, interval, extraLabels, writeMetrics)
}

// InitPush sets up periodic push for metrics from s to the given pushURL with the given interval.
//
// extraLabels may contain comma-separated list of `label="value"` labels, which will be added
// to all the metrics before pushing them to pushURL.
//
/// The metrics are pushed to pushURL in Prometheus text exposition format.
// See https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md#text-based-format
//
// It is recommended pushing metrics to /api/v1/import/prometheus endpoint according to
// https://docs.victoriametrics.com/#how-to-import-data-in-prometheus-exposition-format
//
// It is OK calling InitPush multiple times with different pushURL -
// in this case metrics are pushed to all the provided pushURL urls.
func (s *Set) InitPush(pushURL string, interval time.Duration, extraLabels string) {
	writeMetrics := func(w io.Writer) {
		s.WritePrometheus(w)
	}
	initPush(pushURL, interval, extraLabels, writeMetrics)
}

func initPush(pushURL string, interval time.Duration, extraLabels string, writeMetrics func(w io.Writer)) {
	if interval <= 0 {
		panic(fmt.Errorf("BUG: interval must be positive; got %s", interval))
	}
	if err := validateTags(extraLabels); err != nil {
		panic(fmt.Errorf("BUG: invalid extraLabels=%q: %s", extraLabels, err))
	}
	go func() {
		ticker := time.NewTicker(interval)
		var bb bytes.Buffer
		var tmpBuf []byte
		for range ticker.C {
			bb.Reset()
			writeMetrics(&bb)
			if len(extraLabels) > 0 {
				tmpBuf = addExtraLabels(tmpBuf[:0], bb.Bytes(), extraLabels)
				bb.Reset()
				bb.Write(tmpBuf)
			}
			resp, err := http.Post(pushURL, "text/plain", &bb)
			if err != nil {
				log.Printf("cannot push metrics to %q: %s", pushURL, err)
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				log.Printf("unexpected status code in response from %q: %d; expecting 2xx", pushURL, resp.StatusCode)
				continue
			}
		}
	}()
}

func addExtraLabels(dst, src []byte, extraLabels string) []byte {
	for len(src) > 0 {
		var line []byte
		n := bytes.IndexByte(src, '\n')
		if n >= 0 {
			line = src[:n]
			src = src[n+1:]
		} else {
			line = src
			src = nil
		}
		n = bytes.IndexByte(line, '{')
		if n >= 0 {
			dst = append(dst, line[:n+1]...)
			dst = append(dst, extraLabels...)
			dst = append(dst, ',')
			dst = append(dst, line[n+1:]...)
		} else {
			n = bytes.LastIndexByte(line, ' ')
			if n < 0 {
				panic(fmt.Errorf("BUG: missing whitespace in the generated Prometheus text exposition line %q", line))
			}
			dst = append(dst, line[:n]...)
			dst = append(dst, '{')
			dst = append(dst, extraLabels...)
			dst = append(dst, '}')
			dst = append(dst, line[n:]...)
		}
		dst = append(dst, '\n')
	}
	return dst
}