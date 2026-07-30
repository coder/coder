import { afterEach, describe, expect, it, vi } from "vitest";
import { generateUUID, isUUID } from "./uuid";

describe("generateUUID", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("delegates to crypto.randomUUID when available", () => {
		const uuid = "11111111-1111-4111-8111-111111111111";
		const randomUUID = vi.spyOn(crypto, "randomUUID").mockReturnValue(uuid);

		expect(generateUUID()).toBe(uuid);
		expect(randomUUID).toHaveBeenCalledTimes(1);
	});

	it("returns a valid version 4 UUID via the native path", () => {
		expect(isUUID(generateUUID())).toBe(true);
	});

	describe("fallback (crypto.randomUUID unavailable)", () => {
		afterEach(() => {
			vi.unstubAllGlobals();
		});

		it("sets the version and variant bits regardless of random input", () => {
			// All-zero bytes leave only the version and variant bits that
			// generateUUID sets itself.
			vi.stubGlobal("crypto", {
				getRandomValues: <T extends ArrayBufferView | null>(array: T): T => {
					if (array instanceof Uint8Array) {
						array.fill(0);
					}
					return array;
				},
			});

			const uuid = generateUUID();
			expect(isUUID(uuid)).toBe(true);
			expect(uuid).toBe("00000000-0000-4000-8000-000000000000");
		});

		it("preserves the remaining random bits", () => {
			// All-one bytes verify only the version and variant nibbles are
			// masked (4 and b), leaving every other bit untouched.
			vi.stubGlobal("crypto", {
				getRandomValues: <T extends ArrayBufferView | null>(array: T): T => {
					if (array instanceof Uint8Array) {
						array.fill(0xff);
					}
					return array;
				},
			});

			const uuid = generateUUID();
			expect(isUUID(uuid)).toBe(true);
			expect(uuid).toBe("ffffffff-ffff-4fff-bfff-ffffffffffff");
		});
	});

	it("generates unique values across calls", () => {
		const uuids = new Set(Array.from({ length: 1000 }, () => generateUUID()));
		expect(uuids.size).toBe(1000);
	});
});
