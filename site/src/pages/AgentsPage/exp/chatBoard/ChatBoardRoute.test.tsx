import { screen } from "@testing-library/react";
import { createMemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Experiment } from "#/api/typesGenerated";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import ChatBoardRoute from "./ChatBoardRoute";

const dashboard = vi.hoisted((): { experiments: Experiment[] } => ({
	experiments: [],
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ experiments: dashboard.experiments }),
}));

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
		dashboard.experiments = [];
	});

	it("leaves for /agents until this browser opts in", async () => {
		dashboard.experiments = ["chat-board"];
		const { router } = renderBoardRoute();
		await screen.findByText("agents home");
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("leaves for /agents without the deployment experiment, even if opted in", async () => {
		localStorage.setItem("agents.exp.chat-board", "true");
		const { router } = renderBoardRoute();
		await screen.findByText("agents home");
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("renders the board while on", async () => {
		dashboard.experiments = ["chat-board"];
		localStorage.setItem("agents.exp.chat-board", "true");
		const { router } = renderBoardRoute();
		await screen.findByText("the board");
		expect(router.state.location.pathname).toBe("/agents/board");
	});
});
