import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { BrowserToolbar } from "./BrowserToolbar";

const navState = {
	url: "http://localhost:3000/",
	title: "App",
	loading: false,
	canGoBack: true,
	canGoForward: false,
};

const renderToolbar = () => {
	const handlers = {
		onNavigate: vi.fn(),
		onBack: vi.fn(),
		onForward: vi.fn(),
		onReload: vi.fn(),
	};
	render(
		<BrowserToolbar navState={navState} status="connected" {...handlers} />,
	);
	return handlers;
};

describe("BrowserToolbar", () => {
	it("navigates to the typed address", async () => {
		const user = userEvent.setup();
		const { onNavigate } = renderToolbar();
		const address = screen.getByRole("textbox", { name: "Address" });
		await user.click(address);
		await user.keyboard("localhost:8080/docs{Enter}");
		expect(onNavigate).toHaveBeenCalledWith("localhost:8080/docs");
	});

	it("reverts the address on Escape", async () => {
		const user = userEvent.setup();
		const { onNavigate } = renderToolbar();
		const address = screen.getByRole("textbox", { name: "Address" });
		await user.click(address);
		await user.keyboard("other{Escape}");
		expect(address).toHaveValue(navState.url);
		expect(onNavigate).not.toHaveBeenCalled();
	});

	it("sends history and reload actions", async () => {
		const user = userEvent.setup();
		const { onBack, onForward, onReload } = renderToolbar();
		await user.click(screen.getByRole("button", { name: "Back" }));
		await user.click(screen.getByRole("button", { name: "Forward" }));
		await user.click(screen.getByRole("button", { name: "Reload" }));
		expect(onBack).toHaveBeenCalledOnce();
		// Forward is disabled when there is no forward history.
		expect(onForward).not.toHaveBeenCalled();
		expect(onReload).toHaveBeenCalledOnce();
	});
});
