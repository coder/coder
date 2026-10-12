import { describe, expect, it } from "vitest";
import { webhookOrigin } from "./chatAutomations";

describe("webhookOrigin", () => {
	it.each([
		// A loopback access URL behind a port share: senders use the page.
		[
			"http://127.0.0.1:3100",
			"https://3000--dev--eph--apps.dogfood.cdr.dev",
			"https://3000--dev--eph--apps.dogfood.cdr.dev",
		],
		[
			"http://localhost:3000",
			"https://coder.example.com",
			"https://coder.example.com",
		],
		[
			"http://[::1]:3000",
			"https://coder.example.com",
			"https://coder.example.com",
		],
		// A reachable access URL wins over the address this tab uses.
		[
			"https://coder.example.com",
			"https://10.0.0.5:8080",
			"https://coder.example.com",
		],
		// A port forward to a loopback access URL: only the page's port is
		// reachable from this browser.
		[
			"http://127.0.0.1:3100",
			"http://127.0.0.1:13491",
			"http://127.0.0.1:13491",
		],
	])("uses %s seen from %s as %s", (accessURL, pageOrigin, expected) => {
		expect(webhookOrigin(accessURL, pageOrigin)).toBe(expected);
	});
});
