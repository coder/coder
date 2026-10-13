import http.server
import json
import time

PORT = 8765


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            body = json.dumps({"status": "ok"}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_response(404)
        self.end_headers()


if __name__ == "__main__":
    # Simulates slow start-up work such as compiling assets.
    time.sleep(3)
    server = http.server.HTTPServer(("127.0.0.1", PORT), Handler)
    print(f"serving on http://127.0.0.1:{PORT}", flush=True)
    server.serve_forever()
