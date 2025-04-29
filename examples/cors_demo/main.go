package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
)

var enableCORS bool
var port int

func main() {
	flag.IntVar(&port, "port", 7600, "Port to run the server on")
	flag.BoolVar(&enableCORS, "cors", false, "Enable CORS headers")
	flag.Parse()

	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/test", testHandler)

	fmt.Printf("Server started at :%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	if enableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	tmpl := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Fetch Test</title>
	</head>
	<body>
		<h1>Fetch Tester</h1>
		<input type="text" id="url" placeholder="Enter URL" size="50" value="https://enable-cors.org/" />
		<select id="method">
			<option>GET</option>
			<option>POST</option>
			<option>PUT</option>
			<option>DELETE</option>
		</select>
		<button onclick="sendRequest()">Send Request</button>
		<ul id="log"></ul>

		<script>
			function sendRequest() {
				const url = document.getElementById('url').value;
				const method = document.getElementById('method').value;
				fetch(url, { method: method })
					.then(response => {
						const log = document.getElementById('log');
						const li = document.createElement('li');
						li.textContent = method + " " + url + ": " + response.status;
						log.appendChild(li);
					})
					.catch(error => {
						const log = document.getElementById('log');
						const li = document.createElement('li');
						li.textContent = '(check network log for details) Error: ' + error;
						log.appendChild(li);
					});
			}
		</script>
	</body>
	</html>
	`
	t := template.Must(template.New("page").Parse(tmpl))
	t.Execute(w, nil)
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	if enableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	fmt.Fprintf(w, "You made a %s request to /test", r.Method)
}
