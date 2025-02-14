import { includeOrigin } from "./MobileMenu";

const mockOrigin = "https://example.com";

describe("support link", () => {
	it("should include origin if target starts with '/'", () => {
		(window as unknown as { location: Partial<Location> }).location = {
			origin: mockOrigin,
		}; // Mock the location origin

		expect(includeOrigin("/test")).toBe(mockOrigin + "/test");
		expect(includeOrigin("/path/to/resource")).toBe(
			mockOrigin + "/path/to/resource",
		);
	});

	it("should return the target unchanged if it does not start with '/'", () => {
		expect(includeOrigin(mockOrigin + "/page")).toBe(mockOrigin + "/page");
		expect(includeOrigin("../relative/path")).toBe("../relative/path");
		expect(includeOrigin("relative/path")).toBe("relative/path");
	});
});
