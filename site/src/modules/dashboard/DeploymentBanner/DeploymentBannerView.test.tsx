import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { SessionCountDeploymentStats } from "#/api/typesGenerated";
import { MockDeploymentStats } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { DeploymentBannerView } from "./DeploymentBannerView";

const renderSessionCount = (
	sessionCount: Partial<SessionCountDeploymentStats>,
) =>
	render(
		<DeploymentBannerView
			stats={{
				...MockDeploymentStats,
				session_count: {
					vscode: 0,
					jetbrains: 0,
					ssh: 0,
					reconnecting_pty: 0,
					session_counts: {},
					apps: {},
					...sessionCount,
				},
			}}
		/>,
	);

const connectionLabels = (container: HTMLElement) =>
	within(container)
		.getAllByRole("img", { name: /active connections$/ })
		.map((el) => el.getAttribute("aria-label"));

describe("DeploymentBannerView session counts", () => {
	it("shows the busiest apps first and the rest behind an overflow popover", async () => {
		const { container } = renderSessionCount({
			session_counts: {
				zulu: 2,
				echo: 1,
				delta: 1,
				charlie: 1,
				bravo: 1,
				zero: 0,
				negative: -1,
			},
			apps: { echo: { display_name: "Alpha" } },
		});

		// Descending count, then display name (echo renders as "Alpha").
		expect(connectionLabels(container)).toEqual([
			"zulu: 2 active connections",
			"Alpha: 1 active connections",
			"bravo: 1 active connections",
			"charlie: 1 active connections",
		]);

		await userEvent.click(screen.getByRole("button", { name: "+1 more" }));
		// jsdom has no layout, so hideWhenDetached keeps the popover visually
		// hidden and role queries skip it; find it by label instead.
		const list = within(
			await screen.findByLabelText("More active connections"),
		);
		expect(list.getByText("delta")).toBeInTheDocument();
		expect(list.queryByText("zero")).not.toBeInTheDocument();
	});

	it("renders an empty state", () => {
		renderSessionCount({});
		expect(screen.getByText("No active connections")).toBeInTheDocument();
	});

	it("shows unknown apps by identifier even without app metadata", () => {
		const { container } = renderSessionCount({
			session_counts: { future_ide: 3 },
			apps: undefined as never,
		});
		expect(connectionLabels(container)).toEqual([
			"future_ide: 3 active connections",
		]);
		expect(screen.getByText("future_ide")).toBeInTheDocument();
	});

	it("only renders bundled icons and falls back to the name when one breaks", () => {
		renderSessionCount({
			session_counts: { cursor: 2, evil: 1 },
			apps: {
				cursor: { display_name: "Cursor", icon: "/icon/cursor.svg" },
				evil: { display_name: "Evil", icon: "https://example.com/x.svg" },
			},
		});

		// Icon-only for a bundled icon, generic icon plus name otherwise.
		expect(screen.queryByText("Cursor")).not.toBeInTheDocument();
		expect(screen.getByText("Evil")).toBeInTheDocument();
		expect(screen.queryByRole("img", { name: "Evil icon" })).toBeNull();

		fireEvent.error(screen.getByRole("img", { name: "Cursor icon" }));
		expect(screen.getByText("Cursor")).toBeInTheDocument();
		expect(screen.queryByRole("img", { name: "Cursor icon" })).toBeNull();
	});
});
