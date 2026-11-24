import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import type * as ApiModule from "#/api/api";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import {
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgentStarting,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { organizeAgentApps } from "./AgentApps/AgentApps";
import { AgentRow } from "./AgentRow";

vi.mock("#/api/api", async (importOriginal) => ({
	...(await importOriginal<typeof ApiModule>()),
	watchWorkspaceAgentLogs: vi.fn(() => ({
		addEventListener: vi.fn(),
		close: vi.fn(),
	})),
}));

describe("organizeAgentApps", () => {
	test("returns one ungrouped app", () => {
		const result = organizeAgentApps([{ ...MockWorkspaceApp }]);

		expect(result).toEqual([{ apps: [MockWorkspaceApp] }]);
	});

	test("handles ordering correctly", () => {
		const bugApp = { ...MockWorkspaceApp, slug: "bug", group: "creatures" };
		const birdApp = { ...MockWorkspaceApp, slug: "bird", group: "creatures" };
		const fishApp = { ...MockWorkspaceApp, slug: "fish", group: "creatures" };
		const riderApp = { ...MockWorkspaceApp, slug: "rider" };
		const zedApp = { ...MockWorkspaceApp, slug: "zed" };
		const result = organizeAgentApps([
			bugApp,
			riderApp,
			birdApp,
			zedApp,
			fishApp,
		]);

		expect(result).toEqual([
			{ group: "creatures", apps: [bugApp, birdApp, fishApp] },
			{ apps: [riderApp] },
			{ apps: [zedApp] },
		]);
	});
});

describe("AgentRow", () => {
	it("follows the agent's state until the user toggles the logs", async () => {
		const mockReadyAgent: WorkspaceAgent = {
			...MockWorkspaceAgentStarting,
			lifecycle_state: "ready",
			logs_length: 1,
		};

		const Harness: React.FC = () => {
			const [agent, setAgent] = useState(mockReadyAgent);

			return (
				<>
					<button
						type="button"
						onClick={() =>
							setAgent({ ...agent, lifecycle_state: "start_error" })
						}
					>
						Fail startup
					</button>
					<button
						type="button"
						onClick={() =>
							setAgent({ ...agent, lifecycle_state: "start_timeout" })
						}
					>
						Time out startup
					</button>
					<AgentRow
						agent={agent}
						workspace={MockWorkspace}
						template={MockTemplate}
						initialMetadata={[]}
						onUpdateAgent={vi.fn()}
					/>
				</>
			);
		};

		renderWithAuth(<Harness />);
		const user = userEvent.setup();
		const logsToggle = await screen.findByRole("button", { name: /^Logs/ });
		expect(logsToggle).toHaveAttribute("aria-expanded", "false");

		await user.click(screen.getByRole("button", { name: "Fail startup" }));
		expect(logsToggle).toHaveAttribute("aria-expanded", "true");

		await user.click(logsToggle);
		await user.click(screen.getByRole("button", { name: "Time out startup" }));
		expect(logsToggle).toHaveAttribute("aria-expanded", "false");
	});
});
