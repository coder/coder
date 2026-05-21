import { embedRedirect, retrieveRedirect, sanitizeRedirect } from "./redirect";

describe("redirect helper functions", () => {
	describe("embedRedirect", () => {
		it("embeds the page to return to in the URL", () => {
			const result = embedRedirect("/workspaces", "/page");
			expect(result).toEqual("/page?redirect=%2Fworkspaces");
		});
		it("defaults to navigating to the login page", () => {
			const result = embedRedirect("/workspaces");
			expect(result).toEqual("/login?redirect=%2Fworkspaces");
		});
	});
	describe("retrieveRedirect", () => {
		it("retrieves the page to return to from the URL", () => {
			const result = retrieveRedirect("?redirect=%2Fworkspaces");
			expect(result).toEqual("/workspaces");
		});
	});

	describe("sanitizeRedirect", () => {
		it("is a no-op for a relative path", () => {
			expect(sanitizeRedirect("/bar/baz")).toEqual("/bar/baz");
		});
		it("removes the origin from url", () => {
			expect(sanitizeRedirect("http://www.evil.com/bar/baz")).toEqual(
				"/bar/baz",
			);
		});
		it("preserves search params", () => {
			expect(
				sanitizeRedirect("https://www.example.com/bar?baz=1&quux=2"),
			).toEqual("/bar?baz=1&quux=2");
		});
	});
});
