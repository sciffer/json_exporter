# json_exporter

A generic json exporter for prometheus.io monitoring solution, the exporter will extract all the numeric values from the json and turn them into metrics, booleans will be translated(1 - true, 0 - false) and strings will be ignored. The exporter will try to parse through the whole json and array depths while the metric names will include the hierarchy with _ as seperator. 

json_exporter --help will provide with all the options required to use it.

-j.urls - can contain 1 or a list of urls (seperated with commas).
-j.labels & -j.values - are parallel to one each other (the first label will have the first value, etc..) - same as urls a list of labels and values are comma seperated.

Example of how one can use the exporter to monitor elasticsearch:</br></br>
<p>json_exporter -namespace elasticsearch -j.labels cluster,datacenter -j.values es1,dc1 -j.urls localhost:9200/_cluster/health,localhost:9200/_cluster/stats</p>
