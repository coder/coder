import { screen } from "@testing-library/react";
import { createMemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import ChatBoardRoute from "./ChatBoardRoute";

vi.mock("./ChatBoardPage", () => ({
	default: () => <div>the board</div>,
}));

const renderBoardRoute = () =>
	renderWithRouter(
		createMemoryRouter(
			[
				{ path: "/agents", element: <div>agents home</div> },
				{ path: "/agents/board", element: <ChatBoardRoute /> },
			],
			{ initialEntries: ["/agents/board"] },
		),
	);

describe("ChatBoardRoute", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("leaves for /agents while the board is off", async () => {
		const { router } = renderBoardRoute();
		expect(await screen.findByText("agents home")).toBeInTheDocument();
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("renders the board while on", async () => {
		localStorage.setItem("agents.exp.chat-board", "true");
		const { router } = renderBoardRoute();
		expect(await screen.findByText("the board")).toBeInTheDocument();
		expect(router.state.location.pathname).toBe("/agents/board");
	});
});
