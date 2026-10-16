import { readdirSync } from "node:fs";
import { describe, expect, it } from "vitest";
import icons from "./builtinIcons.json";
import { getBuiltinImageDescription } from "./builtinImages";

describe("built-in image catalog", () => {
	it("describes every shipped icon exactly once, including deprecated variants", () => {
		const sources = icons.flatMap((icon) => icon.sources);
		const shipped = readdirSync("static/icon").map((file) => `/icon/${file}`);
		expect(sources.toSorted()).toEqual(shipped.toSorted());
		for (const icon of icons) {
			expect(icon.name.trim()).not.toBe("");
			expect(icon.description.trim()).not.toBe("");
		}
	});

	it.each([
		["/icon/k8s.svg", "Kubernetes logo"],
		["/icon/k8s.png", "Kubernetes logo"],
		["/icon/code.svg", "Visual Studio Code logo"],
		["/icon/docker-white.svg?color=white#preview", "Docker logo"],
		["/icon/folder.svg", "Folder icon"],
		["/icon/fly.io.svg", "Fly.io logo"],
		["/emojis/1f34c.png", "Banana emoji"],
		["/emojis/2764-fe0f.png?version=2", "Red Heart emoji"],
		["/emojis/2764.png", "Red Heart emoji"],
		["/emojis/1f3f3-fe0f-200d-26a7-fe0f.png", "Transgender Flag emoji"],
		["/emojis/1f44b-1f3ff.png", "Waving Hand: dark skin tone emoji"],
	])("describes %s synchronously", (src, expected) => {
		expect(getBuiltinImageDescription(src)).toBe(expected);
	});

	it.each([
		undefined,
		"",
		"/icon/not-in-the-catalog.svg",
		"/emojis/0000.png",
		"/emojis/spritesheet.png",
		"https://example.com/icon/k8s.svg",
		"//example.com/emojis/1f34c.png",
		"/icon/toString",
	])("leaves unknown or external images to the caller: %s", (src) => {
		expect(getBuiltinImageDescription(src)).toBeUndefined();
	});
});
