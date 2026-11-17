import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import WebSearchSources from "./WebSearchSources";

describe("WebSearchSources", () => {
	it("focuses the first web link on opening and keeps keyboard navigation within the popover", async () => {
		const user = userEvent.setup();
		render(
			<WebSearchSources
				sources={[
					{ url: "ftp://example.com/archive", title: "Archive" },
					{ url: "https://example.com/first", title: "First page" },
					{ url: "https://example.com/last", title: "Last page" },
				]}
			/>,
		);

		const trigger = screen.getByRole("button", { name: "Sources (3)" });
		trigger.focus();
		await user.keyboard("{Enter}");

		const popover = await screen.findByRole("dialog", { name: "Sources" });
		const firstLink = within(popover).getByRole("link", {
			name: /First page/,
		});
		const lastLink = within(popover).getByRole("link", { name: /Last page/ });
		await waitFor(() => expect(firstLink).toHaveFocus());

		await user.tab({ shift: true });
		expect(lastLink).toHaveFocus();

		await user.keyboard("{Escape}");
		await waitFor(() => expect(trigger).toHaveFocus());
	});

	it("falls back to dialog focus when there are no web links", async () => {
		const user = userEvent.setup();
		render(
			<WebSearchSources
				sources={[{ url: "ftp://example.com/archive", title: "Archive" }]}
			/>,
		);

		const trigger = screen.getByRole("button", { name: "Sources (1)" });
		trigger.focus();
		await user.keyboard("{Enter}");

		const popover = await screen.findByRole("dialog", { name: "Sources" });
		await waitFor(() => expect(popover).toHaveFocus());
		await user.keyboard("{Escape}");
		await waitFor(() => expect(trigger).toHaveFocus());
	});
});
