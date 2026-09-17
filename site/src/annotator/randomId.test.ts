import { expect, it } from "vitest";
import { randomId } from "./randomId";

it("mints distinct 32-digit hex ids", () => {
	const ids = new Set(Array.from({ length: 50 }, randomId));
	expect(ids.size).toBe(50);
	for (const id of ids) {
		expect(id).toMatch(/^[0-9a-f]{32}$/);
	}
});
