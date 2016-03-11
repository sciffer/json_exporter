package main

import (
	"encoding/json"
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	helpSuffix = " json_exporter exported metric"
	Version = 0.1
)

// Convert regex string to Map
func regexStr2Map(regexString string) *map[string]*regexp.Regexp {
	regexMap := make(map[string]*regexp.Regexp)
	for _, regex := range strings.Split(regexString, "/") {
		pair := strings.Split(regex, ":")
		if (len(pair) == 2) && (len(pair[0]) > 0) && (len(pair[1]) > 0) {
			regexMap[pair[0]] = regexp.MustCompile(pair[1])
		}
	}
	return &regexMap
}

// Exporter collects Elasticsearch stats from the given server and exports
// them using the prometheus metrics package.
type Exporter struct {
	Urls        []string
	namespace   string
	labels      []string
	labelvalues []string
	mutex       sync.RWMutex
	debug       bool
	nextrefresh time.Time
	interval    time.Duration

	up prometheus.Gauge

	gauges  map[string]*prometheus.GaugeVec
	updated map[string]uint
	exist map[string]uint

	blacklist *regexp.Regexp
	whitelist *regexp.Regexp

	pathlabels map[string]*regexp.Regexp

	client *http.Client
}

// NewExporter returns an initialized Exporter.
func JsonExporter(urls []string, timeout time.Duration, namespace string, labels []string, labelvalues []string, debug bool, blacklist string, whitelist string, refreshinterval time.Duration, pathlabels string, valuelabels string) *Exporter {
	gauges := make(map[string]*prometheus.GaugeVec)
	updated := make(map[string]uint)
	exist := make(map[string]uint)
	var blist, wlist *regexp.Regexp
	if blacklist != "" {
		blist = regexp.MustCompile(blacklist)
	}
	if whitelist != "" {
		wlist = regexp.MustCompile(whitelist)
	}

	// Init our exporter.
	exporter := Exporter{
		Urls:        urls,
		namespace:   namespace,
		labels:      labels,
		labelvalues: labelvalues,
		debug:       debug,
		nextrefresh: time.Now(),
		interval:    refreshinterval,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the json query successful?",
		}),

		gauges:  gauges,
		updated: updated,
		exist: exist,

		blacklist: blist,
		whitelist: wlist,

		pathlabels: *(regexStr2Map(pathlabels)),

		client: &http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					c, err := net.DialTimeout(netw, addr, timeout)
					if err != nil {
						return nil, err
					}
					if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
						return nil, err
					}
					return c, nil
				},
			},
		},
	}

	exporter.collectLabels(regexStr2Map(valuelabels))

	return &exporter
}

// Describe describes all the metrics ever exported by the elasticsearch
// exporter. It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up.Desc()

	for _, g := range e.gauges {
		g.Describe(ch)
	}
}

// Matching metric names against blacklist/whitelist
func (e *Exporter) matchMetric(name string) bool {
	if (e.blacklist != nil && e.blacklist.MatchString(name)) || (e.whitelist != nil && !e.whitelist.MatchString(name)) {
		return false
	} else {
		return true
	}
}

// Match metric name based on regex list - for usage as label value
func (e *Exporter) matchLabel(name string, labelRegex *map[string]*regexp.Regexp) string {
	for k, v := range *labelRegex {
		if v.MatchString(name) {
			return k
		}
	}
	return ""
}

// Adding single gauge metric to the slice
func (e *Exporter) addGauge(name string, value float64, help string) {
	if e.matchMetric(name) {
		if _, exists := e.gauges[name]; !exists {
			e.gauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: e.namespace, Name: name, Help: help}, e.labels)
			e.updated[name] = 0
			e.exist[name] = 0
		}
		e.gauges[name].WithLabelValues(e.labelvalues...).Set(value)
		e.updated[name] += 1
	}
}

// Adding a label to slices
func (e *Exporter) addLabel(name string, value string) {
	e.labels = append(e.labels, name)
	e.labelvalues = append(e.labelvalues, value)
}

// Delete the latest label
func (e *Exporter) delLastLabel() {
	newLastIndex := len(e.labels) - 1
	if newLastIndex >= 0 {
		e.labels = e.labels[:newLastIndex]
		e.labelvalues = e.labelvalues[:newLastIndex]
	}
}

// Extract metrics of generic json interface
// push extracted metrics accordingly (to guages only at the moment)
func (e *Exporter) extractLabel(metric string, jsonInt map[string]interface{}, regexMap *map[string]*regexp.Regexp) {
	newMetric := ""
	for k, v := range jsonInt {
		if len(*regexMap) == 0 {
			return
		}
		if len(metric) > 0 {
			newMetric = metric + "_" + k
		} else {
			newMetric = k
		}
		label := e.matchLabel(newMetric, regexMap)
		if label != "" {
			delete(*regexMap, label)
			if e.debug {
				log.Println("Value label regex match with:", newMetric)
			}
			switch vv := v.(type) {
			case string:
				if e.debug {
					log.Println(newMetric, "is string", vv)
				}
				e.addLabel(label, vv)
			case int:
				if e.debug {
					log.Println(newMetric, "is int =>", vv)
				}
				e.addLabel(label, strconv.Itoa(vv))
			case float64:
				if e.debug {
					log.Println(newMetric, "is float64 =>", vv)
				}
				e.addLabel(label, strconv.FormatFloat(vv, 'E', -1, 64))
			case bool:
				if e.debug {
					log.Println(newMetric, "is bool =>", vv)
				}
				e.addLabel(label, strconv.FormatBool(vv))
			}
		} else {
			switch vv := v.(type) {
			case map[string]interface{}:
				if e.debug {
					log.Println(newMetric, "is hash")
				}
				e.extractLabel(newMetric, vv, regexMap)
			}
		}
	}
}

// Collect labels from all URLs based on label regex list from JSON URL's
func (e *Exporter) collectLabels(regexMap *map[string]*regexp.Regexp) {
	for _, URI := range e.Urls {
		resp, err := e.client.Get(URI)
		if err != nil {
			log.Println("Error while querying Json endpoint:", err)
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println("Failed to read Json response body:", err)
			resp.Body.Close()
			continue
		}

		var allJson map[string]interface{}
		err = json.Unmarshal(body, &allJson)
		if err != nil {
			log.Println("Failed to unmarshal JSON into struct:", err)
			continue
		}

		// Extracrt the metrics from the json interface
		e.extractLabel("", allJson, regexMap)
		if len(*regexMap) == 0 {
			break
		}
	}
}

// Extract metrics of generic json interface
// push extracted metrics accordingly (to guages only at the moment)
func (e *Exporter) extractJson(metric string, jsonInt map[string]interface{}) {
	newMetric := ""
	for k, v := range jsonInt {
		if len(metric) > 0 {
			newMetric = metric + "_" + k
		} else {
			newMetric = k
		}
		label := e.matchLabel(newMetric, &e.pathlabels)
		if label != "" {
			newMetric = label
			e.addLabel(label, k)
		}
		switch vv := v.(type) {
		case string:
			if e.debug {
				log.Println(newMetric, "is string", vv)
			}
			if vv[0] == '{' {
				var stats map[string]interface{}
				err := json.Unmarshal([]byte(vv), &stats)
				if err != nil {
					log.Println("Failed to parse json from string", newMetric)
				} else {
					if e.debug {
						log.Println("Extracting json values from the string in:", newMetric)
					}
					e.extractJson(newMetric, stats)
				}
			}
		case int:
			if e.debug {
				log.Println(newMetric, "is int =>", vv, e.labels)
			}
			e.addGauge(newMetric, float64(vv), newMetric+helpSuffix)
		case float64:
			if e.debug {
				log.Println(newMetric, "is float64 =>", vv, e.labels)
			}
			e.addGauge(newMetric, vv, newMetric+helpSuffix)
		case bool:
			if vv {
				if e.debug {
					log.Println(newMetric, "is bool => 1", e.labels)
				}
				e.addGauge(newMetric, float64(1), newMetric+helpSuffix)
			} else {
				if e.debug {
					log.Println(newMetric, "is bool => 0", e.labels)
				}
				e.addGauge(newMetric, float64(0), newMetric+helpSuffix)
			}
		case map[string]interface{}:
			if e.debug {
				log.Println(newMetric, "is hash", e.labels)
			}
			e.extractJson(newMetric, vv)
		case []interface{}:
			if e.debug {
				log.Println(newMetric, "is an array", e.labels)
			}
			e.extractJsonArray(newMetric, vv)
		default:
			if e.debug {
				log.Println(newMetric, "is of a type I don't know how to handle")
			}
		}
		if label != "" {
			e.delLastLabel()
		}
	}
}

// Extract metrics from json array interface
func (e *Exporter) extractJsonArray(metric string, jsonInt []interface{}) {
	newMetric := ""
	for k, v := range jsonInt {
		if len(metric) > 0 {
			newMetric = metric + "_" + strconv.Itoa(k)
		} else {
			newMetric = strconv.Itoa(k)
		}
		label := e.matchLabel(newMetric, &e.pathlabels)
		if label != "" {
			newMetric = label
			e.addLabel(label, strconv.Itoa(k))
		}
		switch vv := v.(type) {
		case string:
			if e.debug {
				log.Println(newMetric, "is string", vv)
			}
			if vv[0] == '{' {
				var stats map[string]interface{}
				err := json.Unmarshal([]byte(vv), &stats)
				if err != nil {
					log.Println("Failed to parse json from string", newMetric)
				} else {
					e.extractJson(newMetric, stats)
					if e.debug {
						log.Println("Extracting json values from the string in:", newMetric)
					}
				}
			}
		case int:
			if e.debug {
				log.Println(newMetric, "is int =>", vv)
			}
			e.addGauge(newMetric, float64(vv), newMetric+helpSuffix)
		case float64:
			if e.debug {
				log.Println(newMetric, "is int =>", vv)
			}
			e.addGauge(newMetric, vv, newMetric+helpSuffix)
		case bool:
			if vv {
				if e.debug {
					log.Println(newMetric, "is bool => 1")
				}
				e.addGauge(newMetric, float64(1), newMetric+helpSuffix)
			} else {
				if e.debug {
					log.Println(newMetric, "is bool => 0")
				}
				e.addGauge(newMetric, float64(0), newMetric+helpSuffix)
			}
		case map[string]interface{}:
			if e.debug {
				log.Println(newMetric, "is hash")
			}
			e.extractJson(newMetric, vv)
		case []interface{}:
			if e.debug {
				log.Println(newMetric, "is an array")
			}
			e.extractJsonArray(newMetric, vv)
		default:
			if e.debug {
				log.Println(newMetric, "is of a type I don't know how to handle")
			}
		}
		if label != "" {
			e.delLastLabel()
		}
	}
}

// Collect fetches the stats from configured elasticsearch location and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	defer func() { ch <- e.up }()

	if e.nextrefresh.Before(time.Now()) {
		for name, updated := range e.updated {
			if updated < e.exist[name] {
				//delete updated value
				delete(e.updated, name)
				delete(e.exist, name)
				//delete metricvec
				delete(e.gauges, name)
			} else {
				e.exist[name] = e.updated[name]
				//reset value
				e.updated[name] = 0
			}
		}

		for _, URI := range e.Urls {
			resp, err := e.client.Get(URI)
			if err != nil {
				e.up.Set(0)
				log.Println("Error while querying Json endpoint:", err)
				continue
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("Failed to read Json response body:", err)
				e.up.Set(0)
				continue
			}
			resp.Body.Close()

			e.up.Set(1)

			var allStats map[string]interface{}
			err = json.Unmarshal(body, &allStats)
			if err != nil {
				log.Println("Failed to unmarshal JSON into struct:", err)
				continue
			}

			// Extracrt the metrics from the json interface
			e.extractJson("", allStats)
		}
		e.nextrefresh = time.Now().Add(e.interval)
	}
	// Report metrics.

	for _, g := range e.gauges {
		g.Collect(ch)
	}
}

func main() {
	var (
		version       = flag.Bool("version", false, "Print version information.")
		listenAddress = flag.String("web.listen-address", ":9109", "Address to listen on for web interface and telemetry.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		Labels        = flag.String("labels", "", "List of labels (comma seperated).")
		LabelValues   = flag.String("values", "", "List of label values (comma seperated)")
		Timeout       = flag.Duration("timeout", 5*time.Second, "Timeout for trying to get to json URI.")
		interval      = flag.Duration("interval", 1*time.Minute, "Refresh interval for json scraping.")
		namespace     = flag.String("namespace", "json", "Namespace for metrics exported from Json.")
		debug         = flag.Bool("debug", false, "Print debug information")
		blacklist     = flag.String("blacklist", "", "Blacklist regex expression of metric names.")
		whitelist     = flag.String("whitelist", "", "Whitelist regex expression of metric names.")
		valuelabel    = flag.String("valuelabel", "", "Create labels from values using metric-name regex, format: <label1>:<regex1>[/<label2>:<regex2>[/...]].")
		pathlabel     = flag.String("pathlabel", "", "Create labels from path segments with regex match, format: <label1>:<regex1>[/<label2>:<regex2>[/...]].")
	)
	flag.Parse()
	log.Println("json_exporter",Version)
	if *version {
		return
	}
	urls := flag.Args()
	if len(urls) < 1 {
		log.Fatal("Got no URL's, please add use the following syntax to add URL's: json_exporter [options] <URL1>[ <URL2>[ ..<URLn>]]")
	} else {
		log.Println("Got the following Url list", urls)
	}
	//Importing static labels
	labels := []string{}
	labelValues := []string{}
	if len(*Labels) > 0 && len(*LabelValues) > 0 {
		labels = strings.Split(*Labels, ",")
		labelValues = strings.Split(*LabelValues, ",")
		if len(labels) != len(labelValues) {
			log.Fatal("Labels amount does not match value amount!!!")
		}
	}

	exporter := JsonExporter(urls, *Timeout, *namespace, labels, labelValues, *debug, *blacklist, *whitelist, *interval, *pathlabel, *valuelabel)
	prometheus.MustRegister(exporter)

	log.Println("Starting Server:", *listenAddress)
	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Json Exporter</title></head>
             <body>
             <h1>Json Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
