import { defaultPlaceholder } from "./defaultPlaceholder";

describe("defaultPlaceholder", () => {
	it("formats numeric defaults, including zero", () => {
		expect(defaultPlaceholder(13337)).toBe("13337");
		expect(defaultPlaceholder(0)).toBe("0");
	});

	it("formats boolean defaults", () => {
		expect(defaultPlaceholder(false)).toBe("false");
		expect(defaultPlaceholder(true)).toBe("true");
	});

	it("returns non-empty string defaults unchanged", () => {
		expect(defaultPlaceholder("v0.10.0")).toBe("v0.10.0");
	});

	it("returns undefined for empty or absent defaults", () => {
		expect(defaultPlaceholder("")).toBeUndefined();
		expect(defaultPlaceholder(undefined)).toBeUndefined();
		expect(defaultPlaceholder(null)).toBeUndefined();
	});

	it("returns undefined for object and array defaults", () => {
		expect(defaultPlaceholder({ key: "value" })).toBeUndefined();
		expect(defaultPlaceholder(["a", "b"])).toBeUndefined();
	});
});
