import * as http from "node:http";
import {
	type APIRequestContext,
	test as base,
	expect,
	type Page,
} from "@playwright/test";

type FixtureSite = {
	parentOrigin: string;
	previewOrigin: string;
	setPages(parent: string, preview: string): void;
};

async function listen(server: http.Server, port: number, host: string) {
	await new Promise<void>((resolve, reject) => {
		server.once("error", reject);
		server.listen(port, host, () => {
			server.off("error", reject);
			resolve();
		});
	});
}

async function close(server: http.Server) {
	await new Promise<void>((resolve, reject) => {
		server.close((error) => (error ? reject(error) : resolve()));
	});
}

const test = base.extend<{ fixtureSite: FixtureSite }>({
	fixtureSite: async ({ page: _page }, run) => {
		let parentHTML = "";
		let previewHTML = "";
		const serve = (req: http.IncomingMessage, res: http.ServerResponse) => {
			const path = new URL(req.url ?? "/", "http://localhost").pathname;
			const html =
				path === "/annotator-test-parent"
					? parentHTML
					: path === "/annotator-test-app"
						? previewHTML
						: undefined;
			res.writeHead(html ? 200 : 404, {
				"Content-Type": "text/html; charset=utf-8",
			});
			res.end(html ?? "Not found");
		};
		const ipv4 = http.createServer(serve);
		const ipv6 = http.createServer(serve);
		await listen(ipv4, 0, "127.0.0.1");
		try {
			const address = ipv4.address();
			if (!address || typeof address === "string") {
				throw new Error("Fixture server has no TCP address");
			}
			await listen(ipv6, address.port, "::1");
			try {
				await run({
					parentOrigin: `http://127.0.0.1:${address.port}`,
					previewOrigin: `http://localhost:${address.port}`,
					setPages: (parent, preview) => {
						parentHTML = parent;
						previewHTML = preview;
					},
				});
			} finally {
				await close(ipv6);
			}
		} finally {
			await close(ipv4);
		}
	},
});

async function loadPreview(
	page: Page,
	request: APIRequestContext,
	baseURL: string | undefined,
	fixtureSite: FixtureSite,
	options: { scriptOrigin?: string; sibling?: boolean } = {},
) {
	if (!baseURL) {
		throw new Error("The annotator test requires a dashboard URL");
	}
	const dashboardOrigin = fixtureSite.parentOrigin;
	const previewOrigin = fixtureSite.previewOrigin;
	const previewURL = new URL(
		"/annotator-test-app?token=private#fragment",
		previewOrigin,
	);
	const scriptURL = new URL("/annotator.js?v=regression", baseURL).href;
	const scriptResponse = await request.get(scriptURL);
	expect(scriptResponse.status()).toBe(200);
	expect(scriptResponse.headers()["content-type"]).toMatch(
		/^(application|text)\/javascript\b/,
	);
	expect(scriptResponse.headers()["cache-control"] ?? "").not.toContain(
		"immutable",
	);

	const messages: unknown[] = [];
	await page.exposeFunction("recordAnnotation", (message: unknown) => {
		messages.push(message);
	});
	const parentHTML = `<!doctype html><html><head><title>Dashboard fixture</title></head><body>
				<script>window.addEventListener("message", (event) => {
					if (event.source === document.getElementById("preview").contentWindow) {
						window.recordAnnotation(event.data);
					}
				});</script>
				<iframe id="preview" title="App preview" src="${previewURL.href}"></iframe>
				${options.sibling ? '<iframe name="sender" title="Sibling sender" srcdoc="<!doctype html><html><body></body></html>"></iframe>' : ""}
			</body></html>`;
	const previewHTML = `<!doctype html><html><head><title>Preview fixture</title>
				<script>
					window.addEventListener("message", (event) => {
						if (event.data?.type === "coder-annotator:set-picking") {
							window.parent.postMessage({
								type: "fixture:received", probe: event.data.probe,
								origin: event.origin, sourceIsParent: event.source === window.parent,
							}, "*");
						}
					});
					window.addEventListener("load", () => {
						document.getElementById("save").addEventListener("click", () => {
							window.parent.postMessage({type: "fixture:app-click"}, "*");
						});
					});
				</script>
				<script src="${scriptURL}" data-coder-origin="${options.scriptOrigin ?? dashboardOrigin}" defer></script>
				</head><body><button id="save">Save</button></body></html>`;
	fixtureSite.setPages(parentHTML, previewHTML);
	await page.goto(`${dashboardOrigin}/annotator-test-parent`, {
		waitUntil: "load",
	});
	return { messages, dashboardOrigin, previewOrigin };
}

async function sendFromDashboard(
	page: Page,
	probe: string,
	picking: unknown = true,
) {
	await page.evaluate(
		({ probe, picking }) => {
			const frame = document.getElementById("preview");
			if (!(frame instanceof HTMLIFrameElement) || !frame.contentWindow) {
				throw new Error("App preview is unavailable");
			}
			frame.contentWindow.postMessage(
				{ type: "coder-annotator:set-picking", picking, probe },
				new URL(frame.src).origin,
			);
		},
		{ probe, picking },
	);
}

async function startAndStopTrustedPreview(page: Page, messages: unknown[]) {
	await expect
		.poll(() => messages)
		.toContainEqual({ type: "coder-annotator:ready" });
	await sendFromDashboard(page, "bootstrap-control");
	await expect
		.poll(() => messages)
		.toContainEqual({ type: "coder-annotator:state", picking: true });
	const beforeStop = messages.length;
	await page
		.getByTitle("App preview")
		.contentFrame()
		.getByRole("button", { name: "Stop annotating" })
		.click();
	await expect
		.poll(() => messages.slice(beforeStop))
		.toContainEqual({ type: "coder-annotator:state", picking: false });
}

async function expectAppClickAfterProbe(
	page: Page,
	messages: unknown[],
	probe: string,
	origin: string,
	sourceIsParent: boolean,
) {
	await expect
		.poll(() => messages)
		.toContainEqual({
			type: "fixture:received",
			probe,
			origin,
			sourceIsParent,
		});
	await page
		.getByTitle("App preview")
		.contentFrame()
		.getByRole("button", { name: "Save", exact: true })
		.click();
	await expect
		.poll(() => messages)
		.toContainEqual({ type: "fixture:app-click" });
}

// The fixture replaces only the embedding page and the third-party app.
// The injected script is fetched from the real dashboard build.
test("loads the packaged annotator and submits from a cross-origin preview", async ({
	page,
	request,
	baseURL,
	fixtureSite,
}) => {
	const { messages, previewOrigin } = await loadPreview(
		page,
		request,
		baseURL,
		fixtureSite,
	);
	await expect
		.poll(() => messages)
		.toContainEqual({ type: "coder-annotator:ready" });

	await sendFromDashboard(page, "trusted");
	await expect
		.poll(() => messages)
		.toContainEqual({
			type: "coder-annotator:state",
			picking: true,
		});
	const preview = page.getByTitle("App preview").contentFrame();
	await preview.getByRole("button", { name: "Save", exact: true }).click();
	await preview
		.getByRole("textbox", { name: "Annotation comment" })
		.fill("Make this primary");
	await preview
		.getByRole("textbox", { name: "Annotation comment" })
		.press("Enter");

	await expect
		.poll(() => messages)
		.toContainEqual(
			expect.objectContaining({
				type: "coder-annotator:submit",
				page: expect.objectContaining({
					url: `${previewOrigin}/annotator-test-app`,
					title: "Preview fixture",
				}),
				annotations: [
					expect.objectContaining({
						comment: "Make this primary",
						element: expect.objectContaining({
							selector: "#save",
							text: "Save",
						}),
					}),
				],
			}),
		);
});

test("rejects a parent command when the script expects a different origin", async ({
	page,
	request,
	baseURL,
	fixtureSite,
}) => {
	const { messages, dashboardOrigin } = await loadPreview(
		page,
		request,
		baseURL,
		fixtureSite,
		{
			scriptOrigin: "https://other.example.com",
		},
	);
	// This synthetic accepted command proves the bundle mounted even though
	// its mismatched target origin prevents ready from reaching the parent.
	const preview = page.frame({ url: /\/annotator-test-app(?:\?|$)/ });
	if (!preview) {
		throw new Error("App preview is unavailable");
	}
	await preview.evaluate(() => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:set-picking", picking: true },
				origin: "https://other.example.com",
				source: window.parent,
			}),
		);
	});
	await page
		.getByTitle("App preview")
		.contentFrame()
		.getByRole("button", { name: "Stop annotating" })
		.click();

	await sendFromDashboard(page, "wrong-origin");
	await expectAppClickAfterProbe(
		page,
		messages,
		"wrong-origin",
		dashboardOrigin,
		true,
	);
});

test("rejects a same-origin command sent from a sibling frame", async ({
	page,
	request,
	baseURL,
	fixtureSite,
}) => {
	const { messages, dashboardOrigin, previewOrigin } = await loadPreview(
		page,
		request,
		baseURL,
		fixtureSite,
		{
			sibling: true,
		},
	);
	await startAndStopTrustedPreview(page, messages);
	const sibling = page.frame({ name: "sender" });
	if (!sibling) {
		throw new Error("Sibling sender is unavailable");
	}
	await sibling.evaluate((previewOrigin) => {
		const preview = window.parent.frames[0];
		if (!preview) {
			throw new Error("App preview is unavailable");
		}
		preview.postMessage(
			{ type: "coder-annotator:set-picking", picking: true, probe: "sibling" },
			previewOrigin,
		);
	}, previewOrigin);
	await expectAppClickAfterProbe(
		page,
		messages,
		"sibling",
		dashboardOrigin,
		false,
	);
});

test("rejects a malformed command from the trusted parent", async ({
	page,
	request,
	baseURL,
	fixtureSite,
}) => {
	const { messages, dashboardOrigin } = await loadPreview(
		page,
		request,
		baseURL,
		fixtureSite,
	);
	await startAndStopTrustedPreview(page, messages);
	await sendFromDashboard(page, "malformed", "yes");
	await expectAppClickAfterProbe(
		page,
		messages,
		"malformed",
		dashboardOrigin,
		true,
	);
});
