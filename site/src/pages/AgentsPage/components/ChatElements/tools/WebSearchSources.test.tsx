import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
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

	it("restricts source icon requests to bundled assets for exact known web hosts", async () => {
		const user = userEvent.setup();
		renderComponent(
			<WebSearchSources
				sources={[
					"https://GITHUB.com/coder/coder",
					"https://github.com.evil.example/docs",
					"https://coder.com/docs",
					"https://docs.github.com/en",
					"http://docs.aws.amazon.com/",
					"https://linear.app/docs",
					"https://internal.github.com/",
					"https://project.github.io/",
					"https://github.com@internal.example/",
					"https://github.com-example.com/",
					"http://127.0.0.1/private",
					"http://[::1]/private",
					"https://constructor/",
					"/docs/github.com",
					"not a URL",
					"ftp://github.com/release",
					"data:text/html,unsafe",
					"mailto:security@github.com",
				].map((url) => ({ url, title: "" }))}
			/>,
		);

		expect(
			screen.queryAllByAltText("").map((image) => image.getAttribute("src")),
		).toEqual(["/icon/github.svg", "/icon/coder.svg"]);

		await user.click(screen.getByRole("button", { name: "Sources (18)" }));

		expect(
			within(screen.getByRole("dialog", { name: "Sources" }))
				.queryAllByAltText("")
				.map((image) => image.getAttribute("src")),
		).toEqual([
			"/icon/github.svg",
			"/icon/coder.svg",
			"/icon/github.svg",
			"/icon/aws.svg",
			"/icon/linear.svg",
		]);
	});
});
