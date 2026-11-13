import { describe, expect, it } from "vitest";
import { focusFirstVisible } from "./focus";

describe("focusFirstVisible", () => {
	it("skips an element whose ancestor is not displayed when checkVisibility is missing", () => {
		document.body.innerHTML =
			'<div hidden><button id="hidden">Hidden</button></div><button id="shown">Shown</button>';
		const hidden = document.getElementById("hidden");
		const shown = document.getElementById("shown");

		expect(focusFirstVisible([hidden, shown])).toBe(true);
		expect(shown).toHaveFocus();
	});
});
