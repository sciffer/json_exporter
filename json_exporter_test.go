package main

import (
	"testing"
	"encoding/json"
	"net/http"
	"fmt"
	"strings"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"time"
)

var (
	original = map[string]interface{}{
		"value_label":	1,
		"vlabel":	"value2",
		"pathroot":map[string]interface{}{
			"metric3":	4.0,
			"pathvalue1":map[string]interface{}{
				"metric1": 2,
				"pathvalue2":map[string]interface{}{
					"metric2": 3.0,
				},
			},
		},
		"metric4": 5.0,
		"blacklistedMetric": 6,
	}
	expected = []string{
		"test_value_label{label1=\"value1\",valuelabel=\"value2\"} 1",
		"test_path1_metric1{label1=\"value1\",path1=\"pathvalue1\",valuelabel=\"value2\"} 2",
		"test_path2_metric2{label1=\"value1\",path1=\"pathvalue1\",path2=\"pathvalue2\",valuelabel=\"value2\"} 3", 
		"test_pathroot_metric3{label1=\"value1\",valuelabel=\"value2\"} 4",
		"test_metric4{label1=\"value1\",valuelabel=\"value2\"} 5",
	}
/*
test_metric4{label1="value1",valuelabel="value2"} 5
test_path1_metric1{label1="value1",path1="pathvalue1",valuelabel="value2"} 2
test_path1_pathvalue2_metric2{label1="value1",path1="pathvalue1",valuelabel="value2"} 3
test_pathroot_metric3{label1="value1",valuelabel="value2"} 4
test_value_label{label1="value1",valuelabel="value2"} 1
*/
	notexpected = []string{"test_blacklistedMetric"}
	staticLabels = []string{"label1",}
	staticValues = []string{"value1",}
)

func MockupServer(test *testing.T) {
        json, err := json.Marshal(original)
        if err != nil {
                test.Fatal("Failed to marshal original output to json, stopping unit testing.")
        }
        http.HandleFunc("/testmetrics", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, string(json)) })
        test.Fatal(http.ListenAndServe(":9110", nil))
}

func MyExporter(test *testing.T) {
	fmt.Println("Json Exporter called")
        exporter := JsonExporter([]string{"http://localhost:9110/testmetrics"}, 5*time.Second, "test", staticLabels, staticValues, false, "blacklistedMetric", "", 1*time.Minute, "path1:^pathroot_pathvalue1$/path2:^path1_pathvalue2$", "valuelabel:^vlabel$")
        prometheus.MustRegister(exporter)
        http.Handle("/metrics", prometheus.Handler())
        test.Fatal(http.ListenAndServe(":9109", nil))
}

func TestMain(test *testing.T) {
	test.Log("Started test")
	// Mockaup json server
	go MockupServer(test)
	// Start json exporter
	go MyExporter(test)
	// Give enough time for servers to start
	time.Sleep(3 * time.Second)
	// Query json_exporter
	resp, err := http.Get("http://localhost:9109/metrics")
	if err != nil {
		test.Fatal("Failed to query json_exporter")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		test.Fatal("Failed to parse json_exporter response")
	}
        resp.Body.Close()
	// Test json exporter results
	for _, line := range(expected) {
		if !strings.Contains(string(body), line) {
			test.Error("Could not find string:'"+line+"' in json_exporter body!")
		}
	}
	for _, line := range(notexpected) {
		if strings.Contains(string(body), line) {
			test.Error("Could find string:'"+line+"' in json_exporter body - blacklist is not working!")
		}
	}
	test.Log("End of test")
}
