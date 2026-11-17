import { describe, expect, it } from "vitest";
import { previewKey, previewWrapperHTML } from "./PreviewFrame";

describe("previewKey", () => {
	it("identifies file previews by path and port previews by URL path", () => {
		const preview = { agent_name: "main", path: "/home/coder/docs" };
		// A restarted agent gives the path a new preview ID.
		const before = { ...preview, preview_id: "0123abcd" };
		const after = { ...preview, preview_id: "4567ef01" };

		expect(previewKey(before)).toBe(previewKey(after));
		expect(previewKey(preview)).not.toBe(
			previewKey({ ...preview, path: "/home/coder/site" }),
		);
		expect(previewKey({ agent_name: "main", port: 3000 })).not.toBe(
			previewKey({ agent_name: "main", port: 3000, url_path: "/?folder=x" }),
		);
	});
});

describe("previewWrapperHTML", () => {
	it("frames only the preview's origin and escapes its text", () => {
		const url =
			'https://preview--0123abcd--main.apps.example.com/a"b?q=<x>&y=1';
		const title = '"><script>document.title="x"</script>';

		const doc = new DOMParser().parseFromString(
			previewWrapperHTML(url, title),
			"text/html",
		);

		expect(
			doc
				.querySelector('meta[http-equiv="Content-Security-Policy"]')
				?.getAttribute("content"),
		).toBe("frame-src https://preview--0123abcd--main.apps.example.com");
		expect(doc.querySelectorAll("script")).toHaveLength(0);
		const frames = doc.querySelectorAll("iframe");
		expect(frames).toHaveLength(1);
		expect(frames[0].getAttribute("src")).toBe(url);
		expect(frames[0].getAttribute("title")).toBe(title);
	});
});
