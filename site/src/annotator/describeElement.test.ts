import { describe, expect, it } from "vitest";
import {
	buildSelector,
	describeElement,
	describeOpeningTag,
} from "./describeElement";

function render(html: string): void {
	document.body.innerHTML = html;
}

// The functions under test consume raw DOM nodes, so tests locate fixtures
// with the same selectors the annotator would emit. Failing loudly on a
// missing match keeps each case focused on the behavior it asserts.
function pick(selector: string): Element {
	const element = document.body.querySelector(selector);
	if (!element) {
		throw new Error(`no element matches ${selector}`);
	}
	return element;
}

describe("buildSelector", () => {
	it("escapes ids that are not valid identifiers", () => {
		render('<div id="123"><span id="a:b"></span><i id="-"></i></div>');
		for (const id of ["123", "a:b", "-"]) {
			const element = pick(`[id='${id}']`);
			expect(document.querySelector(buildSelector(element))).toBe(element);
		}
	});

	it("prefers a unique id", () => {
		render('<div><button id="save" class="btn">Save</button></div>');
		expect(buildSelector(pick("button"))).toBe("#save");
	});

	it("prefers a unique test id over classes", () => {
		render(
			'<div><button data-testid="save-button" class="btn primary">Save</button></div>',
		);
		expect(buildSelector(pick("button"))).toBe('[data-testid="save-button"]');
	});

	it("uses stable classes and skips generated ones", () => {
		render(
			'<main><nav class="sidebar css-1abcd2e"><a class="link">A</a></nav></main>',
		);
		expect(buildSelector(pick("a"))).toBe("a.link");
	});

	it("falls back to nth-of-type for repeated siblings", () => {
		render(
			'<ul class="list"><li class="item">1</li><li class="item">2</li><li class="item">3</li></ul>',
		);
		expect(buildSelector(pick("li:nth-child(2)"))).toBe(
			"ul.list > li:nth-of-type(2)",
		);
	});

	it("anchors on the nearest unique ancestor id", () => {
		render(
			'<section id="settings"><div><span class="label">Name</span></div></section><section><div><span class="label">Other</span></div></section>',
		);
		expect(buildSelector(pick("#settings .label"))).toBe(
			"#settings > div > span.label",
		);
	});

	it("ignores ids and test ids too long to be worth a selector", () => {
		render(
			`<div id="${"x".repeat(500)}"><p class="note" data-testid="${"t".repeat(500)}">Hi</p></div>`,
		);
		expect(buildSelector(pick("p"))).toBe("p.note");
	});

	it("names the document roots rather than returning nothing", () => {
		render("<p>Hi</p>");
		expect(buildSelector(document.body)).toBe("body");
		expect(buildSelector(document.documentElement)).toBe("html");
	});
});

describe("describeElement", () => {
	it("captures identifying attributes and trimmed text", () => {
		render(
			'<div><button role="button" aria-label="Save changes" class="btn primary" data-testid="save">  Save\n  now </button></div>',
		);
		const described = describeElement(pick("button"));
		expect(described).toMatchObject({
			tag: "button",
			selector: '[data-testid="save"]',
			testId: "save",
			role: "button",
			ariaLabel: "Save changes",
			classes: ["btn", "primary"],
			text: "Save now",
		});
		expect(described.openingTag).toBe(
			'<button role="button" aria-label="Save changes" class="btn primary" data-testid="save">',
		);
		expect(described.reactComponents).toBeUndefined();
	});

	it("reads React component names from the fiber chain", () => {
		render("<div><span>hi</span></div>");
		const span = pick("span");
		function SaveButton() {
			return null;
		}
		function SettingsForm() {
			return null;
		}
		// Enough of a fiber for bippy to accept it. Work tags: 0
		// FunctionComponent, 5 HostComponent, 14 MemoComponent.
		const fakeFiber = (
			tag: number,
			type: unknown,
			parent: object | null,
			memoizedProps: object = {},
		) => ({
			tag,
			type,
			return: parent,
			memoizedProps,
			stateNode: null,
			child: null,
			sibling: null,
			flags: 0,
			alternate: null,
		});
		const root = fakeFiber(5, "div", null);
		const form = fakeFiber(
			14,
			{ $$typeof: Symbol.for("react.memo"), type: SettingsForm },
			root,
		);
		const button = fakeFiber(0, SaveButton, form, {
			variant: "primary",
			onClick: () => {},
			children: 1,
		});
		const fiber = fakeFiber(5, "span", button);
		Object.defineProperty(span, "__reactFiber$abc123", {
			value: fiber,
			enumerable: true,
		});
		const described = describeElement(span);
		expect(described.reactComponents).toEqual(["SaveButton", "SettingsForm"]);
		expect(described.reactProps).toEqual(["variant", "onClick"]);
	});
});

describe("describeElement text capture", () => {
	it("skips text that is not rendered", () => {
		render(
			`<div id="card">Visible<script>{"token":"abc"}</script><style>.x{}</style><template>tpl</template><span hidden>hid</span><span aria-hidden="true">aria</span></div>`,
		);
		expect(describeElement(pick("#card")).text).toBe("Visible");
	});

	it("skips text hidden through CSS", () => {
		render(
			`<div id="menu">Open<ul style="display: none"><li>Collapsed item</li></ul><span style="visibility: hidden">Reserved</span></div>`,
		);
		expect(describeElement(pick("#menu")).text).toBe("Open");
	});

	it("applies the same rules to the element and its ancestors", () => {
		render(
			'<div hidden><p id="inside">inside</p></div><span id="deco" aria-hidden="true">deco</span><p id="gone" style="display: none">gone</p>',
		);
		for (const id of ["inside", "deco", "gone"]) {
			expect(describeElement(pick(`#${id}`)).text, id).toBeUndefined();
		}
	});

	it("stops walking once it has enough text", () => {
		render(`<div id="big">${"<p>word</p>".repeat(10000)}</div>`);
		const described = describeElement(pick("#big"));
		expect(described.text?.length).toBe(120);
		expect(described.text?.endsWith("...")).toBe(true);
	});

	it("skips text inside form controls and editable regions", () => {
		render(
			'<section><h2>Profile</h2><textarea>my private notes</textarea><select><option>Jane Doe</option></select><div contenteditable="true">draft</div><p>Public copy</p></section>',
		);
		expect(describeElement(pick("section")).text).toBe("Profile Public copy");
		expect(describeElement(pick("textarea")).text).toBeUndefined();
	});
});

describe("describeOpeningTag", () => {
	it("keeps labelling ARIA but not value-carrying ARIA", () => {
		render(
			`<div id="slider" aria-label="Volume" aria-valuetext="secret 42" aria-description="user data" aria-expanded="true"></div>`,
		);
		expect(describeOpeningTag(pick("#slider"))).toBe(
			'<div id="slider" aria-label="Volume" aria-expanded="true">',
		);
	});

	it("keeps only locating attributes and strips URL secrets", () => {
		render(
			'<a id="x" class="link" href="/reset?token=abc#frag" data-user-id="42" data-testid="reset" onclick="steal()" style="color:red" title="Reset">go</a>',
		);
		expect(describeOpeningTag(pick("a"))).toBe(
			'<a id="x" class="link" href="/reset" data-testid="reset" title="Reset">',
		);
	});

	it("never includes form values", () => {
		render(
			'<form><input type="hidden" name="csrf" value="s3cret"><input type="text" name="email" value="jane@example.com" placeholder="Email" autocomplete="email"></form>',
		);
		expect(describeOpeningTag(pick("input[name='csrf']"))).toBe(
			'<input type="hidden" name="csrf">',
		);
		expect(describeOpeningTag(pick("input[name='email']"))).toBe(
			'<input type="text" name="email" placeholder="Email">',
		);
	});

	it("escapes attribute values and truncates long ones", () => {
		render("<div></div>");
		const div = pick("div");
		div.setAttribute("aria-label", 'Say "hi" <now>');
		div.setAttribute("class", "a".repeat(200));
		const tag = describeOpeningTag(div);
		expect(tag).toContain('aria-label="Say &quot;hi&quot; &lt;now>"');
		expect(tag.length).toBeLessThan(200);
	});
});
