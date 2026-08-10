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
		it("drops the hash", () => {
			expect(sanitizeRedirect("/foo?a=1#bar")).toEqual("/foo?a=1");
		});
		it("strips the authority of a protocol-relative url", () => {
			expect(sanitizeRedirect("//evil.com/path")).toEqual("/path");
		});
		it("treats backslashes as slashes, not path characters", () => {
			expect(sanitizeRedirect("/\\evil.com")).toEqual("/");
		});
		it("keeps an encoded slash encoded so it stays same-origin", () => {
			expect(sanitizeRedirect("/%2fevil.com")).toEqual("/%2fevil.com");
		});

		// Regression tests for Cure53 CDM-02-001: a URL's pathname can itself
		// start with "//", and a string starting with "//" is a
		// protocol-relative URL when assigned to `location.href`. None of
		// these inputs may produce a redirect that leaves the origin.
		describe("open redirect hardening (CDM-02-001)", () => {
			it("rejects the PoC redirect after query-string decoding", () => {
				// /login?redirect=https://cure53.de/%2fcure53.de is decoded
				// once by URLSearchParams inside retrieveRedirect.
				const redirect = retrieveRedirect(
					"?redirect=https://cure53.de/%2fcure53.de",
				);
				expect(redirect).toEqual("https://cure53.de//cure53.de");
				expect(sanitizeRedirect(redirect)).toEqual("/");
			});
			it("rejects a double-slash pathname in an absolute url", () => {
				expect(sanitizeRedirect("https://cure53.de//cure53.de")).toEqual("/");
			});
			it("rejects a relative path that normalizes to protocol-relative", () => {
				expect(sanitizeRedirect("/.//evil.com")).toEqual("/");
			});
			it("rejects dot-segment traversal that escapes a path prefix", () => {
				expect(sanitizeRedirect("/api/v2/../../..//evil.com")).toEqual("/");
			});
			it("rejects tab characters stripped by the url parser", () => {
				expect(sanitizeRedirect("https://x/\t/evil.com")).toEqual("/");
			});
			it("falls back to / for unparsable urls", () => {
				expect(sanitizeRedirect("http://[invalid")).toEqual("/");
			});
		});
	});
});
