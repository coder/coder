import { describe, expect, it } from "vitest";
import {
	buildSelector,
	describeElement,
	describeOpeningTag,
} from "./describeElement";

function render(html: string): HTMLElement {
	document.body.innerHTML = html;
	return document.body;
}

describe("buildSelector", () => {
	it("escapes ids that are not valid identifiers", () => {
		const body = render(
			'<div id="123"><span id="a:b"></span><i id="-"></i></div>',
		);
		const digits = body.querySelector("[id='123']");
		const colon = body.querySelector("[id='a:b']");
		const dash = body.querySelector("[id='-']");
		if (!digits || !colon || !dash) {
			throw new Error("missing elements");
		}
		for (const el of [digits, colon, dash]) {
			const selector = buildSelector(el);
			expect(document.querySelector(selector)).toBe(el);
		}
	});

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
		expect(described.openingTag).toBe(
			'<button role="button" aria-label="Save changes" class="btn primary" data-testid="save">',
		);
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
		const body = render(
			`<div id="card">Visible<script>{"token":"abc"}</script><style>.x{}</style><template>tpl</template><span hidden>hid</span><span aria-hidden="true">aria</span></div>`,
		);
		const card = body.querySelector("#card");
		if (!card) {
			throw new Error("missing card");
		}
		expect(describeElement(card).text).toBe("Visible");
	});

	it("stops walking once it has enough text", () => {
		const body = render(`<div id="big">${"<p>word</p>".repeat(10000)}</div>`);
		const big = body.querySelector("#big");
		if (!big) {
			throw new Error("missing big");
		}
		const described = describeElement(big);
		expect(described.text?.length).toBe(120);
		expect(described.text?.endsWith("...")).toBe(true);
	});

	it("skips text inside form controls and editable regions", () => {
		const body = render(
			'<section><h2>Profile</h2><textarea>my private notes</textarea><select><option>Jane Doe</option></select><div contenteditable="true">draft</div><p>Public copy</p></section>',
		);
		const section = body.querySelector("section");
		if (!section) {
			throw new Error("missing section");
		}
		expect(describeElement(section).text).toBe("Profile Public copy");
		const textarea = body.querySelector("textarea");
		expect(textarea && describeElement(textarea).text).toBeUndefined();
	});
});

describe("describeOpeningTag", () => {
	it("keeps labelling ARIA but not value-carrying ARIA", () => {
		const body = render(
			`<div id="slider" aria-label="Volume" aria-valuetext="secret 42" aria-description="user data" aria-expanded="true"></div>`,
		);
		const slider = body.querySelector("#slider");
		if (!slider) {
			throw new Error("missing slider");
		}
		expect(describeOpeningTag(slider)).toBe(
			'<div id="slider" aria-label="Volume" aria-expanded="true">',
		);
	});

	it("keeps only locating attributes and strips URL secrets", () => {
		const body = render(
			'<a id="x" class="link" href="/reset?token=abc#frag" data-user-id="42" data-testid="reset" onclick="steal()" style="color:red" title="Reset">go</a>',
		);
		const link = body.querySelector("a");
		expect(link && describeOpeningTag(link)).toBe(
			'<a id="x" class="link" href="/reset" data-testid="reset" title="Reset">',
		);
	});

	it("never includes form values", () => {
		const body = render(
			'<form><input type="hidden" name="csrf" value="s3cret"><input type="text" name="email" value="jane@example.com" placeholder="Email" autocomplete="email"></form>',
		);
		const [hidden, email] = Array.from(body.querySelectorAll("input"));
		expect(describeOpeningTag(hidden)).toBe(
			'<input type="hidden" name="csrf">',
		);
		expect(describeOpeningTag(email)).toBe(
			'<input type="text" name="email" placeholder="Email">',
		);
	});

	it("escapes attribute values and truncates long ones", () => {
		const body = render("<div></div>");
		const div = body.querySelector("div");
		if (!div) {
			throw new Error("missing div");
		}
		div.setAttribute("aria-label", 'Say "hi" <now>');
		div.setAttribute("class", "a".repeat(200));
		const tag = describeOpeningTag(div);
		expect(tag).toContain('aria-label="Say &quot;hi&quot; &lt;now>"');
		expect(tag.length).toBeLessThan(200);
	});
});
