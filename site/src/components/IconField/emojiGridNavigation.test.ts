import { enableEmojiGridNavigation } from "./emojiGridNavigation";

const perLine = 3;

const createEmoji = (label: string) => {
	const button = document.createElement("button");
	button.type = "button";
	button.tabIndex = -1;
	button.setAttribute("aria-label", label);
	return button;
};

/** Builds a minimal copy of the emoji-mart shadow DOM structure. */
const setup = (rows: number[], options: { hiddenRows?: number[] } = {}) => {
	const host = document.createElement("div");
	document.body.appendChild(host);
	const root = host.attachShadow({ mode: "open" });

	const search = document.createElement("input");
	search.type = "search";
	root.appendChild(search);

	const scroll = document.createElement("div");
	scroll.className = "scroll";
	root.appendChild(scroll);

	const category = document.createElement("div");
	category.className = "category";
	const body = document.createElement("div");
	body.className = "relative";
	category.appendChild(body);
	scroll.appendChild(category);

	rows.forEach((count, rowIndex) => {
		const row = document.createElement("div");
		row.className = "flex row";
		if (!options.hiddenRows?.includes(rowIndex)) {
			for (let i = 0; i < count; i++) {
				row.appendChild(createEmoji(`${rowIndex}-${i}`));
			}
		}
		body.appendChild(row);
	});

	const cleanup = enableEmojiGridNavigation(root);
	const emoji = (label: string) =>
		root.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
	const focused = () => root.activeElement?.getAttribute("aria-label");
	const press = (key: string) =>
		root.activeElement?.dispatchEvent(
			new KeyboardEvent("keydown", { key, bubbles: true, composed: true }),
		);

	return { root, scroll, body, search, emoji, focused, press, cleanup };
};

afterEach(() => {
	document.body.innerHTML = "";
});

describe("enableEmojiGridNavigation", () => {
	it("moves focus between emojis with the arrow keys", async () => {
		const { emoji, focused, press } = setup([perLine, perLine]);
		emoji("0-0").focus();

		press("ArrowRight");
		await vi.waitFor(() => expect(focused()).toBe("0-1"));
		press("ArrowDown");
		await vi.waitFor(() => expect(focused()).toBe("1-1"));
		press("ArrowLeft");
		await vi.waitFor(() => expect(focused()).toBe("1-0"));
		press("ArrowUp");
		await vi.waitFor(() => expect(focused()).toBe("0-0"));
	});

	it("wraps left and right across rows", async () => {
		const { emoji, focused, press } = setup([perLine, perLine]);
		emoji("0-2").focus();

		press("ArrowRight");
		await vi.waitFor(() => expect(focused()).toBe("1-0"));
		press("ArrowLeft");
		await vi.waitFor(() => expect(focused()).toBe("0-2"));
	});

	it("jumps to the row edges with Home and End", async () => {
		const { emoji, focused, press } = setup([perLine]);
		emoji("0-1").focus();

		press("End");
		await vi.waitFor(() => expect(focused()).toBe("0-2"));
		press("Home");
		await vi.waitFor(() => expect(focused()).toBe("0-0"));
	});

	it("clamps to the last emoji of a shorter row", async () => {
		const { emoji, focused, press } = setup([perLine, 1]);
		emoji("0-2").focus();

		press("ArrowDown");
		await vi.waitFor(() => expect(focused()).toBe("1-0"));
	});

	it("waits for a virtualized row to render before focusing it", async () => {
		const { body, emoji, focused, press } = setup([perLine, perLine], {
			hiddenRows: [1],
		});
		emoji("0-0").focus();

		press("ArrowDown");
		const row = body.children[1];
		row.appendChild(createEmoji("1-0"));
		row.appendChild(createEmoji("1-1"));
		await vi.waitFor(() => expect(focused()).toBe("1-0"));
	});

	it("skips rows inside a hidden category list", async () => {
		const { scroll, emoji, focused, press } = setup([perLine]);
		const hidden = document.createElement("div");
		hidden.style.display = "none";
		const category = document.createElement("div");
		category.className = "category";
		const row = document.createElement("div");
		row.className = "flex row";
		row.appendChild(createEmoji("hidden-0"));
		category.appendChild(row);
		hidden.appendChild(category);
		scroll.appendChild(hidden);
		emoji("0-0").focus();

		press("ArrowDown");
		await new Promise((resolve) => setTimeout(resolve, 50));
		expect(focused()).toBe("0-0");
	});

	it("makes the grid scroller a tab stop", () => {
		const { scroll } = setup([perLine]);
		expect(scroll.tabIndex).toBe(0);
	});

	it("stops handling keys after cleanup", async () => {
		const { emoji, focused, press, cleanup } = setup([perLine]);
		cleanup();
		emoji("0-0").focus();

		press("ArrowRight");
		await new Promise((resolve) => setTimeout(resolve, 50));
		expect(focused()).toBe("0-0");
	});
});
