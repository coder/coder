import { normalizeActionURL } from "./InboxItem";

describe("normalizeActionURL", () => {
	it("strips origin from same-origin absolute URL", () => {
		const url = `${window.location.origin}/deployment/users?filter=status%3Aactive`;
		expect(normalizeActionURL(url)).toBe(
			"/deployment/users?filter=status%3Aactive",
		);
	});

	it("preserves cross-origin absolute URL", () => {
		const url = "https://external.example.com/some/path";
		expect(normalizeActionURL(url)).toBe(url);
	});

	it("returns relative paths unchanged", () => {
		expect(normalizeActionURL("/deployment/users")).toBe("/deployment/users");
	});

	it("preserves hash fragments", () => {
		const url = `${window.location.origin}/settings#section`;
		expect(normalizeActionURL(url)).toBe("/settings#section");
	});

	it("handles empty string", () => {
		expect(normalizeActionURL("")).toBe("/");
	});
});
