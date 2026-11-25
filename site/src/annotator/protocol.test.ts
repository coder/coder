import { describe, expect, it } from "vitest";
import {
	type AnnotationSubmission,
	type AnnotatorToHostMessage,
	annotatorQueryParam,
	type HostToAnnotatorMessage,
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

const submission: AnnotatorToHostMessage = {
	type: "coder-annotator:submit",
	page: {
		url: "https://app.example.com/settings?token=secret#frag",
		title: "App",
		viewport: { width: 800, height: 600 },
	},
	annotations: [{ id: "a", comment: "Make it red", element }],
};

// Most cases exercise the submit branch, so narrow once here and let them
// assert on what the parser kept or rewrote.
function parseSubmission(value: unknown): AnnotationSubmission {
	const parsed = parseAnnotatorToHostMessage(value);
	if (parsed?.type !== "coder-annotator:submit") {
		throw new Error(`expected a submit message, got ${parsed?.type}`);
	}
	return parsed;
}

// The proxy looks for the same name; see AnnotationQueryParam in
// coderd/workspaceapps/proxy.go.
it("names the marker the proxy strips", () => {
	expect(annotatorQueryParam).toBe("coder_annotate");
});

describe("parseAnnotatorToHostMessage", () => {
	it("keeps only the origin and path of the page URL", () => {
		expect(parseSubmission(submission).page.url).toBe(
			"https://app.example.com/settings",
		);
	});

	it("rejects submissions without a usable page URL", () => {
		expect(
			parseAnnotatorToHostMessage({
				...submission,
				// oxlint-disable-next-line eslint/no-script-url -- Deliberately hostile input exercises scheme rejection.
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

	it("only accepts ids shaped like the overlay's own", () => {
		const withId = (id: unknown) =>
			parseAnnotatorToHostMessage({
				...submission,
				annotations: [{ id, comment: "x", element }],
			});
		expect(withId("0".repeat(32))).toBeDefined();
		expect(withId("a")).toBeDefined();
		// Ids key dashboard-side maps: prototype names must not get through.
		for (const id of [
			"__proto__",
			"constructor",
			"toString",
			"a b",
			'x"]',
			"",
			"3b241101-e2bb-4255-8caf-4136c566a962",
			"f".repeat(65),
		]) {
			expect(withId(id), id).toBeUndefined();
		}
	});

	it("truncates fields, drops non-string array items, and clamps numbers", () => {
		const [annotation] = parseSubmission({
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
		}).annotations;
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
		const many = parseSubmission({
			...submission,
			annotations: Array.from({ length: 50 }, (_, i) => ({
				id: String(i),
				comment: "x",
				element,
			})),
		});
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
	});

	it("treats a state message without a picking flag as idle", () => {
		expect(
			parseAnnotatorToHostMessage({ type: "coder-annotator:state" }),
		).toEqual({ type: "coder-annotator:state", picking: false });
	});
});

describe("isHostToAnnotatorMessage", () => {
	it("requires a boolean picking flag", () => {
		const picking: HostToAnnotatorMessage = {
			type: "coder-annotator:set-picking",
			picking: true,
		};
		expect(isHostToAnnotatorMessage(picking)).toBe(true);
		expect(isHostToAnnotatorMessage({ ...picking, picking: "yes" })).toBe(
			false,
		);
	});
});
