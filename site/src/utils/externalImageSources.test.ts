import {
	externalImageHost,
	isDeploymentIconPath,
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
		["javascript:alert(1)", true],
		["file:///etc/passwd", true],
		["ftp://attacker.example.com/img.png", true],
	])("isExternalImageSource(%j) === %j", (src, expected) => {
		expect(isExternalImageSource(src)).toBe(expected);
	});
});

describe("isDeploymentIconPath", () => {
	it.each([
		["", false],
		["/emojis/1f4bb.png", true],
		["/icon/aws.svg", true],
		["/icon/aws.svg?v=2", true],
		["https://example.com/icon.png", false],
		["//example.com/icon.png", false],
		["/\\example.com/icon.png", false],
		["javascript:alert(1)", false],
		["data:image/png;base64,xxx", false],
		["icon/aws.svg", false],
	])("isDeploymentIconPath(%j) === %j", (value, expected) => {
		expect(isDeploymentIconPath(value)).toBe(expected);
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
