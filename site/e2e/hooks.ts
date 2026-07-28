import type { BrowserContext, Page } from "@playwright/test";
import { coderPort, gitAuth } from "./constants";

export const beforeCoderTest = (page: Page) => {
	page.on("console", (msg) => {
		const location = msg.location();
		// Filters out a bunch of junk warnings the browser produces.
		if (!location.url) {
			return;
		}
		// Filters out the gigantic CODER logo we print on every page load, as well
		// as some other noise.
		if (msg.type() === "info") {
			return;
		}
		console.info(`[console][${msg.type()}] ${msg.text()}`);
	});

	page.on("response", async (response) => {
		// Don't log responses for static assets.
		if (!isApiCall(response.url())) {
			return;
		}
		// Don't log successful responses. Those are almost always less interesting.
		if (response.ok()) {
			return;
		}

		let responseText: string;
		try {
			responseText = await response.text();
			responseText = responseText.replaceAll("\n", "");
		} catch {
			responseText = "<n/a>";
		}

		console.info(
			`[response] url=${response.url()} status=${response.status()} body=${responseText}`,
		);
	});

	page.on("popup", async (popup) => {
		console.info(`[popup] url=${popup.url()}`);
	});

	page.on("pageerror", async (error) => {
		console.error("[pageerror]", error);
	});

	page.on("crash", async (page) => {
		console.error("[crash]", page.url());
	});
};

export const resetExternalAuthKey = async (context: BrowserContext) => {
	// Find the session token so we can destroy the external auth links between
	// tests, to ensure valid authentication happens each time.
	const cookies = await context.cookies();
	const sessionCookie = cookies.find((c) => c.name === "coder_session_token");

	// Reset every provider the suite exercises so repeated runs against the same
	// coderd instance get a clean slate; otherwise the device-flow link from a
	// previous iteration trips the test on its next pass.
	const providers = [gitAuth.webProvider, gitAuth.deviceProvider];
	await Promise.all(
		providers.map((provider) =>
			deleteExternalAuthLink(provider, sessionCookie?.value),
		),
	);
};

const deleteExternalAuthLink = async (
	provider: string,
	sessionToken: string | undefined,
): Promise<void> => {
	const response = await fetch(
		`http://127.0.0.1:${coderPort}/api/v2/external-auth/${provider}?coder_session_token=${sessionToken}`,
		{ method: "DELETE" },
	);

	// 200 = link deleted; 404 = no link existed for this provider.
	if (response.status !== 200 && response.status !== 404) {
		const data = await response.text();
		console.error("failed to delete external auth link", data);
		throw new Error(
			`failed to delete external auth link: HTTP response ${response.status}`,
		);
	}
};

const isApiCall = (urlString: string): boolean => {
	const url = new URL(urlString);
	const apiPath = "/api/v2";

	return url.pathname.startsWith(apiPath);
};
