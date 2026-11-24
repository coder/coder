import { waitFor } from "@testing-library/react";
import { API } from "#/api/api";
import { MockDeploymentStats } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { DeploymentBanner } from "./DeploymentBanner";

describe("DeploymentBanner", () => {
	it("refetches the stats when the server says they update", async () => {
		const collectedAt = new Date().toISOString();
		const getStats = vi.spyOn(API, "getDeploymentStats").mockResolvedValue({
			...MockDeploymentStats,
			collected_at: collectedAt,
			next_update_at: collectedAt,
		});

		renderWithAuth(<DeploymentBanner />);

		await waitFor(() => expect(getStats).toHaveBeenCalledTimes(2), {
			timeout: 3000,
		});
	});
});
