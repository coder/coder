import { enableEmojiGridNavigation } from "./emojiGridNavigation";

/**
 * Builds a minimal emoji-mart shadow DOM whose search input highlights the
 * next emoji on ArrowRight, like the library does.
 */
const setup = () => {
	const host = document.createElement("div");
	document.body.appendChild(host);
	const root = host.attachShadow({ mode: "open" });
	root.innerHTML = `
		<input type="search" />
		<div class="scroll"><div class="category">
			<button aria-label="a"></button><button aria-label="b"></button>
		</div></div>`;
	const emojis = Array.from(root.querySelectorAll("button"));
	root.querySelector("input")?.addEventListener("keydown", (event) => {
		if (event.key !== "ArrowRight") {
			return;
		}
		const index = emojis.findIndex((e) => e.hasAttribute("aria-selected"));
		emojis[index]?.removeAttribute("aria-selected");
		emojis[index + 1]?.setAttribute("aria-selected", "true");
	});
	const cleanup = enableEmojiGridNavigation(root);
	const focused = () => root.activeElement?.getAttribute("aria-label");
	const press = (key: string) =>
		root.activeElement?.dispatchEvent(
			new KeyboardEvent("keydown", { key, bubbles: true, composed: true }),
		);
	return { emojis, focused, press, cleanup };
};

afterEach(() => {
	document.body.innerHTML = "";
});

describe("enableEmojiGridNavigation", () => {
	it("moves focus to the emoji the picker highlights", async () => {
		const { emojis, focused, press } = setup();
		emojis[0].setAttribute("aria-selected", "true");
		emojis[0].focus();

		press("ArrowRight");
		await vi.waitFor(() => expect(focused()).toBe("b"));
	});

	it("stops handling keys after cleanup", async () => {
		const { emojis, focused, press, cleanup } = setup();
		cleanup();
		emojis[0].setAttribute("aria-selected", "true");
		emojis[0].focus();

		press("ArrowRight");
		await new Promise((resolve) => setTimeout(resolve, 50));
		expect(focused()).toBe("a");
	});
});
