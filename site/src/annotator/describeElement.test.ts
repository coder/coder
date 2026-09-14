import { buildSelector, describeElement } from "./describeElement";

function render(html: string): HTMLElement {
	document.body.innerHTML = html;
	return document.body;
}

describe("buildSelector", () => {
	it("prefers a unique id", () => {
		const body = render(
			'<div><button id="save" class="btn">Save</button></div>',
		);
		const button = body.querySelector("button");
		expect(button && buildSelector(button)).toBe("#save");
	});

	it("prefers a unique test id over classes", () => {
		const body = render(
			'<div><button data-testid="save-button" class="btn primary">Save</button></div>',
		);
		const button = body.querySelector("button");
		expect(button && buildSelector(button)).toBe('[data-testid="save-button"]');
	});

	it("uses stable classes and skips generated ones", () => {
		const body = render(
			'<main><nav class="sidebar css-1abcd2e"><a class="link">A</a></nav></main>',
		);
		const link = body.querySelector("a");
		expect(link && buildSelector(link)).toBe("a.link");
	});

	it("falls back to nth-of-type for repeated siblings", () => {
		const body = render(
			'<ul class="list"><li class="item">1</li><li class="item">2</li><li class="item">3</li></ul>',
		);
		const items = body.querySelectorAll("li");
		expect(buildSelector(items[1])).toBe("ul.list > li:nth-of-type(2)");
	});

	it("anchors on the nearest unique ancestor id", () => {
		const body = render(
			'<section id="settings"><div><span class="label">Name</span></div></section><section><div><span class="label">Other</span></div></section>',
		);
		const label = body.querySelector("#settings .label");
		expect(label && buildSelector(label)).toBe("#settings > div > span.label");
	});
});

describe("describeElement", () => {
	it("captures identifying attributes and trimmed text", () => {
		const body = render(
			'<div><button role="button" aria-label="Save changes" class="btn primary" data-testid="save">  Save\n  now </button></div>',
		);
		const button = body.querySelector("button");
		if (!button) {
			throw new Error("missing button");
		}
		const described = describeElement(button);
		expect(described).toMatchObject({
			tag: "button",
			selector: '[data-testid="save"]',
			testId: "save",
			role: "button",
			ariaLabel: "Save changes",
			classes: ["btn", "primary"],
			text: "Save now",
		});
		expect(described.html.startsWith("<button")).toBe(true);
		expect(described.reactComponents).toBeUndefined();
	});

	it("reads React component names from the fiber chain", () => {
		const body = render("<div><span>hi</span></div>");
		const span = body.querySelector("span");
		if (!span) {
			throw new Error("missing span");
		}
		function SaveButton() {
			return null;
		}
		function SettingsForm() {
			return null;
		}
		const fiber = {
			type: "span",
			return: {
				type: SaveButton,
				_debugSource: { fileName: "src/SettingsForm.tsx", lineNumber: 42 },
				return: {
					type: { $$typeof: Symbol.for("react.memo"), type: SettingsForm },
					return: { type: "div", return: null },
				},
			},
		};
		Object.defineProperty(span, "__reactFiber$abc123", {
			value: fiber,
			enumerable: true,
		});
		const described = describeElement(span);
		expect(described.reactComponents).toEqual(["SaveButton", "SettingsForm"]);
		expect(described.sourceLocation).toBe("src/SettingsForm.tsx:42");
	});
});
