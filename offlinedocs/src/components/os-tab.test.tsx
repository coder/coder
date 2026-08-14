import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";
import { Tab } from "fumadocs-ui/components/tabs";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { OSTab, detectOS } from "./os-tab";

// These tests render OSTab against the real fumadocs-ui Tabs in a jsdom DOM
// (set up by the test/jsdom-setup.mjs preload), so they exercise the actual
// coupling between OSTab's shared-group clamp and fumadocs' group store rather
// than a reimplementation of it. document/window/sessionStorage here are the
// jsdom globals.

let container: HTMLElement;
let root: Root;

beforeEach(() => {
	window.sessionStorage.clear();
	window.localStorage.clear();
	container = document.createElement("div");
	document.body.appendChild(container);
	root = createRoot(container);
});

afterEach(async () => {
	await act(async () => {
		root.unmount();
	});
	container.remove();
});

async function renderOSTab(items: string[]) {
	await act(async () => {
		root.render(
			<OSTab items={items}>
				{items.map((os) => (
					<Tab key={os} value={os}>
						{`${os} panel`}
					</Tab>
				))}
			</OSTab>,
		);
	});
}

function activePanelText(): string | undefined {
	const active = container.querySelector(
		'[role="tabpanel"][data-state="active"]',
	);
	return active?.textContent?.trim();
}

function panelStates() {
	return [...container.querySelectorAll('[role="tabpanel"]')].map((panel) => ({
		state: panel.getAttribute("data-state"),
		text: (panel.textContent ?? "").trim(),
	}));
}

test("OSTab keeps a non-empty panel when the stored OS is not in the set", async () => {
	// A sibling set stored `linux` in the shared `os` group; this macOS/Windows
	// set does not offer it. Before the clamp, fumadocs applied `linux`, matched
	// no panel, and rendered an empty box. The set must fall back to its own
	// default (the first item) and show a real panel.
	window.sessionStorage.setItem("os", "linux");
	await renderOSTab(["macOS", "Windows"]);

	assert.equal(
		activePanelText(),
		"macOS panel",
		`expected the default panel to stay active, got: ${JSON.stringify(
			panelStates(),
		)}`,
	);
});

test("OSTab adopts the shared OS when the set offers it", async () => {
	// The stored OS is one this set offers, so the shared group value wins over
	// the default first tab. This is the behavior the clamp must not break.
	window.sessionStorage.setItem("os", "windows");
	await renderOSTab(["Linux", "macOS", "Windows"]);

	assert.equal(
		activePanelText(),
		"Windows panel",
		`expected the stored OS to be adopted, got: ${JSON.stringify(
			panelStates(),
		)}`,
	);
});

test("detectOS maps Linux, macOS, and Windows user agents to an offered label", () => {
	// detectOS is UA-driven and pure once the UA is injected, so the three regexes
	// are asserted directly rather than through a browser. Both jsdom render tests
	// preseed storage and never reach this branch.
	const all = ["macOS", "Linux", "Windows"];
	assert.equal(
		detectOS(all, "Mozilla/5.0 (X11; Linux x86_64) Gecko Firefox/130.0"),
		"Linux",
	);
	assert.equal(
		detectOS(all, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari"),
		"macOS",
	);
	assert.equal(
		detectOS(all, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome"),
		"Windows",
	);
});

test("detectOS returns undefined when the OS is not offered or is unknown", () => {
	// The detected OS (Linux) is not one this set offers, so it falls through and
	// the component keeps its default tab.
	assert.equal(detectOS(["macOS", "Windows"], "X11; Linux x86_64"), undefined);
	// iOS is not treated as macOS.
	assert.equal(
		detectOS(
			["macOS"],
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
		),
		undefined,
	);
	// An empty UA (server render) detects nothing.
	assert.equal(detectOS(["macOS", "Windows", "Linux"], ""), undefined);
});

test("OSTab selects a real panel on a first visit with empty storage", async () => {
	// Neither test above exercises the first-visit branch (both preseed storage).
	// With empty storage the component runs detectOS and either seeds the detected
	// OS (when this set offers it) or keeps its default first tab; either way a
	// real, non-empty offered panel is active, never the empty box.
	await renderOSTab(["macOS", "Windows"]);
	const active = activePanelText();
	assert.ok(
		active === "macOS panel" || active === "Windows panel",
		`expected a real offered panel to be active, got: ${JSON.stringify(
			panelStates(),
		)}`,
	);
});
