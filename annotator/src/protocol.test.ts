import { describe, expect, it } from "vitest";
import {
	isHostToAnnotatorMessage,
	parseAnnotatorToHostMessage,
} from "./protocol";

const element = {
	tag: "button",
	selector: "#save",
	classes: ["btn"],
	openingTag: '<button id="save">',
	rect: { x: 1, y: 2, width: 3, height: 4 },
};

const submission = {
	type: "coder-annotator:submit",
	page: {
		url: "https://app.example.com/settings?token=secret#frag",
		title: "App",
		viewport: { width: 800, height: 600 },
	},
	annotations: [{ id: "a", comment: "Make it red", element }],
};

describe("parseAnnotatorToHostMessage", () => {
	it("keeps only the origin and path of the page URL", () => {
		const parsed = parseAnnotatorToHostMessage(submission);
		expect(parsed?.type).toBe("coder-annotator:submit");
		if (parsed?.type !== "coder-annotator:submit") {
			throw new Error("unexpected type");
		}
		expect(parsed.page.url).toBe("https://app.example.com/settings");
	});

	it("rejects submissions without a usable page URL", () => {
		expect(
			parseAnnotatorToHostMessage({
				...submission,
				page: { ...submission.page, url: "javascript:alert(1)" },
			}),
		).toBeUndefined();
		expect(
			parseAnnotatorToHostMessage({
				...submission,
				page: { ...submission.page, url: 42 },
			}),
		).toBeUndefined();
	});

	it("drops annotations whose element lacks identifying fields", () => {
		expect(
			parseAnnotatorToHostMessage({
				...submission,
				annotations: [
					{ id: "a", comment: "x", element: { ...element, selector: "" } },
				],
			}),
		).toBeUndefined();
		expect(
			parseAnnotatorToHostMessage({
				...submission,
				annotations: [{ comment: "x", element }],
			}),
		).toBeUndefined();
	});

	it("truncates fields, drops non-string array items, and clamps numbers", () => {
		const parsed = parseAnnotatorToHostMessage({
			...submission,
			annotations: [
				{
					id: "a",
					comment: "c".repeat(5000),
					element: {
						...element,
						text: "t".repeat(5000),
						classes: [1, "ok", null, "x".repeat(5000)],
						rect: { x: 1e308, y: -1e308, width: Number.NaN, height: 2.6 },
					},
				},
			],
		});
		if (parsed?.type !== "coder-annotator:submit") {
			throw new Error("unexpected type");
		}
		const [annotation] = parsed.annotations;
		expect(annotation.comment).toHaveLength(2000);
		expect(annotation.element.text).toHaveLength(300);
		expect(annotation.element.classes).toEqual(["ok", "x".repeat(300)]);
		expect(annotation.element.rect).toEqual({
			x: 100_000,
			y: -100_000,
			width: 0,
			height: 3,
		});
	});

	it("caps the number of annotations and the total size", () => {
		const many = parseAnnotatorToHostMessage({
			...submission,
			annotations: Array.from({ length: 50 }, (_, i) => ({
				id: String(i),
				comment: "x",
				element,
			})),
		});
		if (many?.type !== "coder-annotator:submit") {
			throw new Error("unexpected type");
		}
		expect(many.annotations).toHaveLength(5);

		const huge = parseAnnotatorToHostMessage({
			...submission,
			annotations: Array.from({ length: 5 }, (_, i) => ({
				id: String(i),
				comment: "x".repeat(2000),
				element: {
					...element,
					text: "t".repeat(300),
					classes: Array.from({ length: 20 }, () => "c".repeat(300)),
				},
			})),
		});
		expect(huge).toBeUndefined();
	});

	it("ignores unknown and malformed messages", () => {
		expect(parseAnnotatorToHostMessage(null)).toBeUndefined();
		expect(parseAnnotatorToHostMessage({ type: "other" })).toBeUndefined();
		expect(
			parseAnnotatorToHostMessage({ type: "coder-annotator:state" }),
		).toEqual({ type: "coder-annotator:state", picking: false });
	});
});

describe("isHostToAnnotatorMessage", () => {
	it("requires a boolean picking flag", () => {
		expect(
			isHostToAnnotatorMessage({
				type: "coder-annotator:set-picking",
				picking: true,
			}),
		).toBe(true);
		expect(
			isHostToAnnotatorMessage({
				type: "coder-annotator:set-picking",
				picking: "yes",
			}),
		).toBe(false);
	});
});
