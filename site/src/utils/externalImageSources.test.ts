import {
	externalImageHost,
	isExternalImageSource,
} from "./externalImageSources";

describe("isExternalImageSource", () => {
	// jsdom serves tests from http://localhost/.
	it.each([
		["", false],
		["   ", false],
		["/emojis/1f4bb.png", false],
		["relative/path.png", false],
		["./relative.png", false],
		["data:image/png;base64,iVBORw0KGgo=", false],
		["blob:http://localhost/1234-5678", false],
		[`${location.origin}/icon/aws.svg`, false],
		["https://attacker.example.com/img.png", true],
		["http://attacker.example.com/img.png", true],
		["HTTPS://ATTACKER.EXAMPLE.COM/img.png", true],
		["  https://attacker.example.com/img.png  ", true],
		["//attacker.example.com/img.png", true],
		["/\\attacker.example.com/img.png", true],
		["\\\\attacker.example.com\\img.png", true],
		["file:///etc/passwd", true],
		["ftp://attacker.example.com/img.png", true],
	])("isExternalImageSource(%j) === %j", (src, expected) => {
		expect(isExternalImageSource(src)).toBe(expected);
	});
});

describe("externalImageHost", () => {
	it("returns the hostname for absolute URLs", () => {
		expect(externalImageHost("https://cdn.example.com/a.png")).toBe(
			"cdn.example.com",
		);
	});

	it("returns undefined for unparsable sources", () => {
		expect(externalImageHost("https://[")).toBeUndefined();
	});
});
