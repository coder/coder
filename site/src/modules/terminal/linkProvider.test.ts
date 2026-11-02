import { type ILink, init, Terminal } from "ghostty-web";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import "#/testHelpers/canvas";
import { createLinkProvider } from "./linkProvider";

const findLink = async (
	terminal: Terminal,
	onOpenLink: (uri: string) => void,
): Promise<ILink> => {
	const provider = createLinkProvider(terminal, onOpenLink);
	const links = await new Promise<ILink[] | undefined>((resolve) =>
		provider.provideLinks(0, resolve),
	);
	expect(links).toHaveLength(1);
	const [link] = links ?? [];
	if (!link) {
		throw new Error("expected a link on the first row");
	}
	return link;
};

describe("createLinkProvider", () => {
	let terminal: Terminal;

	beforeAll(async () => {
		await init();
	});

	afterEach(() => {
		terminal.dispose();
	});

	const openTerminal = (text: string) => {
		terminal = new Terminal();
		terminal.open(document.createElement("div"));
		terminal.write(text);
	};

	it("opens detected URLs through the given handler", async () => {
		openTerminal("visit https://example.com/some/path now");
		const onOpenLink = vi.fn();

		const link = await findLink(terminal, onOpenLink);
		link.activate(new MouseEvent("click"));

		expect(onOpenLink).toHaveBeenCalledWith("https://example.com/some/path");
	});

	it("does not open a link when text is selected", async () => {
		openTerminal("visit https://example.com/some/path now");
		const onOpenLink = vi.fn();

		const link = await findLink(terminal, onOpenLink);
		terminal.select(0, 0, 10);
		link.activate(new MouseEvent("click"));

		expect(onOpenLink).not.toHaveBeenCalled();
	});
});
