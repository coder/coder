// Static file server for a production frontend build plus a transparent
// reverse proxy to the local Coder API, including WebSocket upgrades.
//
// The production bundle talks to the API over same-origin relative URLs
// (`/api/...`). Serving the bundle from a different port therefore
// requires proxying both HTTP and WS to coderd. The session token is
// injected as a header so the page needs no login flow.
import fs from "node:fs";
import http from "node:http";
import net from "node:net";
import path from "node:path";

const PORT = Number(process.env.PERF_WEB_PORT ?? 8082);
const API_TARGET = process.env.CODER_API_URL ?? "http://127.0.0.1:3000";
const API_URL = new URL(API_TARGET);
const TOKEN = process.env.CODER_SESSION_TOKEN;
const ROOT = process.env.PERF_WEB_ROOT;
if (!ROOT) {
	throw new Error("PERF_WEB_ROOT must point at the built site/out directory");
}

const MIME = {
	".html": "text/html; charset=utf-8",
	".js": "text/javascript; charset=utf-8",
	".mjs": "text/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".svg": "image/svg+xml",
	".png": "image/png",
	".jpg": "image/jpeg",
	".jpeg": "image/jpeg",
	".gif": "image/gif",
	".webp": "image/webp",
	".ico": "image/x-icon",
	".woff": "font/woff",
	".woff2": "font/woff2",
	".ttf": "font/ttf",
	".map": "application/json; charset=utf-8",
	".mp3": "audio/mpeg",
	".mp4": "video/mp4",
	".wasm": "application/wasm",
	".txt": "text/plain; charset=utf-8",
	".xml": "application/xml",
};

const sendFile = (res, filePath) => {
	fs.readFile(filePath, (err, data) => {
		if (err) {
			res.writeHead(404).end("not found");
			return;
		}
		res.writeHead(200, {
			"Content-Type": MIME[path.extname(filePath)] ?? "application/octet-stream",
			"Cache-Control": "no-store",
		});
		res.end(data);
	});
};

// Resolves a request path to a file inside ROOT, falling back to
// index.html so client-side routes deep-link correctly.
const resolveStatic = (urlPath) => {
	const clean = decodeURIComponent(urlPath.split("?")[0]);
	const candidate = path.join(ROOT, clean);
	const normalized = path.normalize(candidate);
	if (!normalized.startsWith(ROOT)) {
		return null;
	}
	if (fs.existsSync(normalized) && fs.statSync(normalized).isFile()) {
		return normalized;
	}
	return path.join(ROOT, "index.html");
};

const apiHeaders = (headers) => {
	const next = { ...headers, host: API_URL.host };
	if (TOKEN) {
		next["coder-session-token"] = TOKEN;
	}
	return next;
};

const server = http.createServer((req, res) => {
	if (req.url?.startsWith("/api/") || req.url?.startsWith("/swagger")) {
		const upstream = http.request(
			{
				hostname: API_URL.hostname,
				port: API_URL.port,
				path: req.url,
				method: req.method,
				headers: apiHeaders(req.headers),
			},
			(upRes) => {
				res.writeHead(upRes.statusCode ?? 502, upRes.headers);
				upRes.pipe(res);
			},
		);
		upstream.on("error", () => res.writeHead(502).end("upstream error"));
		req.pipe(upstream);
		return;
	}
	const file = resolveStatic(req.url ?? "/");
	if (!file) {
		res.writeHead(400).end("bad path");
		return;
	}
	sendFile(res, file);
});

// Raw TCP relay for WebSocket upgrades, so `/api/.../stream` works.
server.on("upgrade", (req, socket, head) => {
	const upstream = net.connect(
		Number(API_URL.port || 80),
		API_URL.hostname,
		() => {
			const headers = [
				`GET ${req.url} HTTP/1.1`,
				`Host: ${API_URL.host}`,
				"Connection: Upgrade",
				"Upgrade: websocket",
				`Sec-WebSocket-Version: ${req.headers["sec-websocket-version"]}`,
				`Sec-WebSocket-Key: ${req.headers["sec-websocket-key"]}`,
				`Origin: ${API_TARGET}`,
			];
			if (TOKEN) {
				headers.push(`Coder-Session-Token: ${TOKEN}`);
			}
			if (req.headers["sec-websocket-protocol"]) {
				headers.push(
					`Sec-WebSocket-Protocol: ${req.headers["sec-websocket-protocol"]}`,
				);
			}
			upstream.write(`${headers.join("\r\n")}\r\n\r\n`);
			if (head?.length) {
				upstream.write(head);
			}
			upstream.pipe(socket);
			socket.pipe(upstream);
		},
	);
	upstream.on("error", () => socket.destroy());
	socket.on("error", () => upstream.destroy());
});

server.listen(PORT, "127.0.0.1", () => {
	console.log(`serving ${ROOT} on http://127.0.0.1:${PORT} proxying ${API_TARGET}`);
});
