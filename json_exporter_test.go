package json_exporter_test

import (
	"testing"
	"json"
)

const (
	original map[string]interface{} = {
		"value_label":	1.0,
		"vlabel":	"value2",
		"pathroot":{
			"metric3":	4.0,
			"pathvalue1":{
				"metric1": "pathvalue1",
				"pathvalue3":{
					"metric2": 3.0
				}
			}
		},
		"metric4": 5.0
	}
	expected []string = [
		"test_value_label{label1=\"value1\",valuelabel=\"value2\"} 1",
		"test_path1_metric1{label1=\"value1\",valuelabel=\"value2\",path=\"pathvalue1\"} 2",
		"test_path2_metric2{label1=\"value1\",valuelabel=\"value2\",path=\"pathvalue1\",path2=\"pathvalue2\"} 3", 
		"test_pathroot_metric3{label1=\"value1\",valuelabel=\"value2\"} 4"
		"test_metric4label1=\"value1\",valuelabel=\"value2\"} 5"
	]
	staticLabels []string = ["label1"]
	staticValues []string = ["value1"]
)

func TestUnit(test *testing.T) {
	test.Log("Started test")
	test.Log("End of test")
}
