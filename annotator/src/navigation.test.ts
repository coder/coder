import { describe, expect, it } from "vitest";
import { carryMarkerAcrossNavigation, withMarker } from "./navigation";

const location = {
	href: "https://app.example.com/settings?tab=general",
	origin: "https://app.example.com",
	pathname: "/settings",
	search: "?tab=general",
};

describe("withMarker", () => {
	it("adds the marker to same-origin document navigations", () => {
		expect(withMarker("/billing", location)).toBe(
			"https://app.example.com/billing?coder_annotate=1",
		);
		expect(withMarker("/billing?plan=pro", location)).toBe(
			"https://app.example.com/billing?plan=pro&coder_annotate=1",
		);
	});

	it("leaves other origins, fragments and marked links alone", () => {
		expect(withMarker("https://other.example.com/", location)).toBeUndefined();
		expect(withMarker("#section", location)).toBeUndefined();
		expect(withMarker("/settings?tab=general#x", location)).toBeUndefined();
		expect(withMarker("/billing?coder_annotate=1", location)).toBeUndefined();
		expect(withMarker("javascript:void(0)", location)).toBeUndefined();
	});
});

describe("carryMarkerAcrossNavigation", () => {
	it("rewrites plain clicks on same-origin links only", () => {
		document.body.innerHTML = `
			<a id="same" href="/next">next</a>
			<a id="blank" href="/next" target="_blank">new tab</a>
			<a id="download" href="/file" download>file</a>
			<a id="other" href="https://other.example.com/">other</a>
		`;
		const stop = carryMarkerAcrossNavigation(document, window);
		const click = (id: string, init: MouseEventInit = {}) => {
			const anchor = document.getElementById(id) as HTMLAnchorElement;
			anchor.dispatchEvent(
				new MouseEvent("click", { bubbles: true, cancelable: true, ...init }),
			);
			// jsdom does not navigate, so the rewritten href is observable.
			return new URL(anchor.href).searchParams.get("coder_annotate");
		};
		expect(click("same")).toBe("1");
		expect(click("blank")).toBeNull();
		expect(click("download")).toBeNull();
		expect(click("other")).toBeNull();

		const modified = document.getElementById("same") as HTMLAnchorElement;
		modified.href = "/again";
		expect(click("same", { metaKey: true })).toBeNull();

		stop();
		modified.href = "/later";
		expect(click("same")).toBeNull();
	});
});
