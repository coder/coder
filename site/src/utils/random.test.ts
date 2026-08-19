import { describe, expect, it } from "vitest";
import { generateConnectionSessionId, generateUUID } from "#/utils/random";
import { isUUID } from "#/utils/uuid";

describe("generateUUID", () => {
	it("outputs a valid UUID", () => {
		expect(isUUID(generateUUID())).toBe(true);
	});

	it("generates unique values across calls", () => {
		const numValues = 1000;
		const ids = new Set(
			Array.from({ length: numValues }, () => generateUUID()),
		);
		expect(ids.size).toBe(numValues);
	});

	it("produces a valid UUID without crypto.randomUUID (insecure context)", () => {
		const descriptor = Object.getOwnPropertyDescriptor(crypto, "randomUUID");
		Object.defineProperty(crypto, "randomUUID", {
			configurable: true,
			value: undefined,
		});
		try {
			expect(isUUID(generateUUID())).toBe(true);
		} finally {
			if (descriptor) {
				Object.defineProperty(crypto, "randomUUID", descriptor);
			}
		}
	});
});

describe("generateConnectionSessionId", () => {
	it("is 32 characters long", () => {
		const id = generateConnectionSessionId();
		expect(id.length).toBe(32);
	});

	it("outputs a hexadecimal string", () => {
		const id = generateConnectionSessionId();
		const hexRegex = /^[\da-f]+$/;
		expect(hexRegex.test(id)).toBe(true);
	});

	it("generates unique values across calls", () => {
		const numValues = 1000;
		const ids = new Set(
			Array.from({ length: numValues }, () => generateConnectionSessionId()),
		);
		expect(ids.size).toBe(numValues);
	});
});
