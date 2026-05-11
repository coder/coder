import { screen } from "@testing-library/react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ScriptsChart } from "./ScriptsChart";
import type { Stage } from "./StagesChart";

const stage: Stage = {
	name: "init",
	label: "init",
	section: "Agent scripts",
	tooltip: {
		heading: "Agent scripts",
		description: "Agent scripts",
	},
};

describe("ScriptsChart", () => {
	it("renders timed out and unknown script statuses", () => {
		renderComponent(
			<ScriptsChart
				stage={stage}
				onBack={vi.fn()}
				timings={[
					{
						name: "wait for service",
						status: "timed_out",
						exitCode: 124,
						range: {
							startedAt: new Date("2024-01-01T00:00:00Z"),
							endedAt: new Date("2024-01-01T00:00:03Z"),
						},
					},
					{
						name: "new status script",
						status: "cancelled_by_user",
						exitCode: 1,
						range: {
							startedAt: new Date("2024-01-01T00:00:03Z"),
							endedAt: new Date("2024-01-01T00:00:04Z"),
						},
					},
				]}
			/>,
		);

		expect(screen.getByText("timed out")).toBeInTheDocument();
		expect(screen.getByText("cancelled_by_user")).toBeInTheDocument();
	});
});
