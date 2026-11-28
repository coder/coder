import { beforeEach, describe, expect, it } from "vitest";
import { pickTarget } from "./pickTarget";

const host = document.createElement("div");
host.attachShadow({ mode: "open" }).append(document.createElement("i"));

// A composed path only exists while an event is being dispatched, so the
// pick happens inside a capturing window listener, as in the overlay.
function pickDuringClick(target: Node): Element | null {
	let picked: Element | null | undefined;
	const listener = (event: Event) => {
		picked = pickTarget(host, event);
	};
	window.addEventListener("click", listener, true);
	target.dispatchEvent(
		new MouseEvent("click", { bubbles: true, composed: true }),
	);
	window.removeEventListener("click", listener, true);
	if (picked === undefined) {
		throw new Error("the click never reached the window");
	}
	return picked;
}

describe("pickTarget", () => {
	beforeEach(() => {
		document.body.innerHTML = "<main><p>Hi <b>there</b></p></main>";
		document.body.append(host);
	});

	it("resolves text nodes to their element", () => {
		const bold = document.querySelector("b");
		if (!bold?.firstChild) {
			throw new Error("missing fixture");
		}
		expect(pickDuringClick(bold.firstChild)).toBe(bold);
	});

	it("ignores the overlay's own events and the document roots", () => {
		const own = host.shadowRoot?.firstChild;
		if (!own) {
			throw new Error("missing fixture");
		}
		expect(pickDuringClick(own)).toBeNull();
		expect(pickDuringClick(document.body)).toBeNull();
		expect(pickDuringClick(document.documentElement)).toBeNull();
	});
});
