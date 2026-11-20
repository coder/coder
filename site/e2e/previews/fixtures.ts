import * as http from "node:http";
import type { AddressInfo } from "node:net";
import escapeHTML from "lodash/escape";

/** Requests the preview page sends, which must reach only its own host. */
export const requestVectors = [
	"fetch",
	"xhr",
	"img",
	"script",
	"preload",
	"prefetch",
	"form",
	"repost",
	"ping",
	"beacon",
	"eventsource",
	"websocket",
	"iframe",
] as const;
type RequestVector = (typeof requestVectors)[number];

/** Ways out of the wrapper, which must fail even toward the preview's host. */
export const escapeVectors = ["popup", "top", "parent"] as const;
type EscapeVector = (typeof escapeVectors)[number];

/** A request that reached a fixture server through coderd. */
type Hit = { method: string; path: string };

type FixtureServer = {
	port: number;
	hits: Hit[];
	paths: () => string[];
	close: () => Promise<void>;
};

const listen = async (
	handler: http.RequestListener,
	hits: Hit[],
): Promise<FixtureServer> => {
	const server = http.createServer((req, res) => {
		hits.push(hitOf(req));
		handler(req, res);
	});
	// WebSocket handshakes arrive as upgrades, not requests. Recording them
	// is enough, so the socket closes without a handshake.
	server.on("upgrade", (req, socket) => {
		hits.push(hitOf(req));
		socket.destroy();
	});
	await new Promise<void>((resolve, reject) => {
		server.once("error", reject);
		server.listen(0, "127.0.0.1", resolve);
	});
	return {
		port: (server.address() as AddressInfo).port,
		hits,
		paths: () => hits.map((hit) => hit.path),
		close: () =>
			new Promise((resolve) => {
				server.closeAllConnections();
				server.close(() => resolve());
			}),
	};
};

const hitOf = (req: http.IncomingMessage): Hit => ({
	method: req.method ?? "",
	path: new URL(req.url ?? "/", "http://fixture").pathname,
});

/**
 * startCanaryApp serves a workspace app that records every request coderd
 * proxies to it. The browser never learns its port, so every hit came through
 * coderd with the user's credentials.
 */
export const startCanaryApp = (): Promise<FixtureServer> =>
	listen((_req, res) => {
		res.writeHead(200, { "Content-Type": "text/html" });
		res.end("<!doctype html><title>Canary</title><h1>Canary app</h1>");
	}, []);

/**
 * attackFromPreview runs in the hostile preview page, which inlines its
 * source, so it must not reference anything outside its own body. It tries
 * each request vector against the canary origin in its `canary` query
 * parameter and then against its own origin, so a hit on its own server shows
 * the canary attempt finished first. It posts what it saw to /report.
 */
const attackFromPreview = async (): Promise<void> => {
	const canaryOrigin = new URLSearchParams(location.search).get("canary");
	if (!canaryOrigin) {
		throw new Error("The attack page needs a canary query parameter.");
	}
	const ownOrigin = location.origin;
	const outcomes: Record<string, string> = {};

	const add = <K extends keyof HTMLElementTagNameMap>(
		tag: K,
		props: Partial<HTMLElementTagNameMap[K]>,
	): HTMLElementTagNameMap[K] => {
		const element = Object.assign(document.createElement(tag), props);
		document.body.append(element);
		return element;
	};
	const loaded = (element: HTMLElement) =>
		new Promise<string>((resolve) => {
			element.addEventListener("load", () => resolve("load"));
			element.addEventListener("error", () => resolve("error"));
		});
	let sinks = 0;
	const sink = () =>
		add("iframe", { name: `sink${sinks++}`, hidden: true }).name;
	const attempt = async (key: string, run: () => Promise<string> | string) => {
		const timeout = new Promise<string>((resolve) => {
			setTimeout(() => resolve("no outcome within 5s"), 5000);
		});
		try {
			outcomes[key] = await Promise.race([run(), timeout]);
		} catch (error) {
			outcomes[key] = `threw ${error}`;
		}
	};

	// Only the href setter of another origin's location is reachable, so
	// these use it rather than assign().
	const escapes: Record<EscapeVector, (url: string) => string> = {
		popup: (url) => (window.open(url) ? "opened" : "blocked"),
		top: (url) => {
			if (window.top) {
				window.top.location.href = url;
			}
			return "assigned";
		},
		parent: (url) => {
			window.parent.location.href = url;
			return "assigned";
		},
	};
	for (const [name, run] of Object.entries(escapes)) {
		for (const [label, origin] of [
			["canary", canaryOrigin],
			["self", ownOrigin],
		]) {
			await attempt(`${name} ${label}`, () => run(`${origin}/v/${name}`));
		}
	}

	await attempt("service-worker self", async () => {
		await navigator.serviceWorker.register("/v/service-worker");
		return "registered";
	});

	const requests: Record<
		RequestVector,
		(url: string) => Promise<string> | string
	> = {
		fetch: async (url) =>
			(await fetch(url, { credentials: "include", mode: "no-cors" })).type,
		xhr: (url) =>
			new Promise((resolve) => {
				const xhr = new XMLHttpRequest();
				xhr.withCredentials = true;
				xhr.open("GET", url);
				xhr.onloadend = () => resolve(`status ${xhr.status}`);
				xhr.send();
			}),
		img: (url) => loaded(add("img", { src: url })),
		script: (url) => loaded(add("script", { src: url })),
		preload: (url) =>
			loaded(add("link", { rel: "preload", as: "image", href: url })),
		prefetch: (url) => loaded(add("link", { rel: "prefetch", href: url })),
		form: (url) => {
			add("form", { method: "post", action: url, target: sink() }).submit();
			return "submitted";
		},
		repost: (url) => {
			const action = `/repost?to=${encodeURIComponent(url)}`;
			add("form", { method: "post", action, target: sink() }).submit();
			return "submitted";
		},
		ping: (url) => {
			add("a", { href: "/landing", ping: url, target: sink() }).click();
			return "clicked";
		},
		beacon: (url) => String(navigator.sendBeacon(url, "beacon")),
		eventsource: (url) =>
			new Promise((resolve) => {
				const source = new EventSource(url, { withCredentials: true });
				source.onopen = () => {
					source.close();
					resolve("open");
				};
				source.onerror = () => {
					source.close();
					resolve("error");
				};
			}),
		websocket: (url) =>
			new Promise((resolve) => {
				const socket = new WebSocket(url.replace(/^http/, "ws"));
				socket.onopen = () => {
					socket.close();
					resolve("open");
				};
				socket.onerror = () => resolve("error");
			}),
		iframe: (url) => loaded(add("iframe", { src: url })),
	};
	for (const [name, run] of Object.entries(requests)) {
		for (const [label, origin] of [
			["canary", canaryOrigin],
			["self", ownOrigin],
		]) {
			await attempt(`${name} ${label}`, () => run(`${origin}/v/${name}`));
		}
	}

	await fetch("/report", { method: "POST", body: JSON.stringify(outcomes) });
};

type HostileServer = FixtureServer & {
	/** Resolves with what the attack page saw of each attempt. */
	report: Promise<Record<string, string>>;
};

/**
 * startHostilePreview serves the page an agent would write to break out of
 * its preview. Its root runs every attack against the origin in its `canary`
 * query parameter and against its own origin, whose hits prove the browser
 * really sent that kind of request. Every other path serves a page that names
 * the path.
 */
export const startHostilePreview = async (): Promise<HostileServer> => {
	let resolveReport: (outcomes: Record<string, string>) => void = () => {};
	const report = new Promise<Record<string, string>>((resolve) => {
		resolveReport = resolve;
	});
	const html = (res: http.ServerResponse, body: string) => {
		res.writeHead(200, { "Content-Type": "text/html" });
		res.end(`<!doctype html>${body}`);
	};

	const server = await listen((req, res) => {
		const url = new URL(req.url ?? "/", "http://fixture");
		switch (url.pathname) {
			case "/": {
				const canaryOrigin = url.searchParams.get("canary");
				// Preload hints make the browser send requests before the page
				// runs, so they come from the response itself.
				const links = [
					`<${canaryOrigin}/v/early-hints>; rel=preload; as=image`,
					"</v/early-hints>; rel=preload; as=image",
				];
				res.writeEarlyHints({ link: links });
				res.setHeader("Link", [
					`<${canaryOrigin}/v/link-header>; rel=preload; as=image`,
					"</v/link-header>; rel=preload; as=image",
				]);
				html(
					res,
					"<title>Hostile preview</title><h1>Hostile preview fixture</h1>" +
						`<script>(${attackFromPreview})()</script>`,
				);
				return;
			}
			case "/repost":
				res.writeHead(307, { Location: url.searchParams.get("to") ?? "/" });
				res.end();
				return;
			case "/report": {
				let body = "";
				req.on("data", (chunk) => {
					body += chunk;
				});
				req.on("end", () => {
					resolveReport(JSON.parse(body));
					res.writeHead(204);
					res.end();
				});
				return;
			}
			case "/v/script":
				res.writeHead(200, { "Content-Type": "text/javascript" });
				res.end("");
				return;
			case "/v/eventsource":
				res.writeHead(200, { "Content-Type": "text/event-stream" });
				res.end("data: hello\n\n");
				return;
			default:
				html(
					res,
					`<title>Preview path</title><h1>Preview path ${escapeHTML(url.pathname + url.search)}</h1>`,
				);
		}
	}, []);
	return { ...server, report };
};
