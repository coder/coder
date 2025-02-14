import { includeOrigin } from "./MobileMenu";

describe("support link", () => {
	it("should include origin if target starts with '/'", () => {
		const mockOrigin = "https://example.com";

		// eslint-disable-next-line window object
		delete (window as any).location; // Remove the existing location object
		// eslint-disable-next-line window object
		(window as any).location = { origin: mockOrigin }; // Mock the location origin

		expect(includeOrigin("/test")).toBe("https://example.com/test");
		expect(includeOrigin("/path/to/resource")).toBe(
			"https://example.com/path/to/resource",
		);
	});

	it("should return the target unchanged if it does not start with '/'", () => {
		expect(includeOrigin("https://example.com/page")).toBe(
			"https://example.com/page",
		);
		expect(includeOrigin("../relative/path")).toBe("../relative/path");
		expect(includeOrigin("relative/path")).toBe("relative/path");
	});
});
