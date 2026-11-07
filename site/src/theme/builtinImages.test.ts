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

	it("describes every shipped emoji PNG", () => {
		const missing = readdirSync("static/emojis")
			.filter((file) => /^[0-9a-f-]+\.png$/.test(file))
			.filter((file) => !getBuiltinImageDescription(`/emojis/${file}`));
		expect(missing).toEqual([]);
	});

	it.each([
		["/icon/k8s.svg", "Kubernetes logo"],
		["/icon/k8s.png", "Kubernetes logo"],
		["/icon/code.svg", "Visual Studio Code logo"],
		["/icon/docker-white.svg?color=white#preview", "Docker logo"],
		["/icon/folder.svg", "Folder icon"],
		["/icon/fly.io.svg", "Fly.io logo"],
		["/emojis/1f34c.png", "banana emoji"],
		["/emojis/2764-fe0f.png?version=2", "red heart emoji"],
		["/emojis/2764.png", "red heart emoji"],
		["/emojis/1f3f3-fe0f-200d-26a7-fe0f.png", "transgender flag emoji"],
		["/emojis/1f44b-1f3ff.png", "waving hand: dark skin tone emoji"],
		["/emojis/1f34b-200d-1f7e9.png", "lime emoji"],
		["/emojis/0031-fe0f-20e3.png", "keycap: 1 emoji"],
		[
			"/emojis/1faf1-1f3fb-200d-1faf2-1f3ff.png",
			"handshake: light skin tone, dark skin tone emoji",
		],
	])("describes %s synchronously", (src, expected) => {
		expect(getBuiltinImageDescription(src)).toBe(expected);
	});

	it.each([
		undefined,
		"",
		"/icon/not-in-the-catalog.svg",
		"/emojis/0000.png",
		"/emojis/1fae9.png",
		"/emojis/sheet-15.1.2.png",
		"/emojis/spritesheet.png",
		"https://example.com/icon/k8s.svg",
		"//example.com/emojis/1f34c.png",
		"/icon/toString",
	])("leaves unknown or external images to the caller: %s", (src) => {
		expect(getBuiltinImageDescription(src)).toBeUndefined();
	});
});
