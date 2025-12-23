import { screen } from "@testing-library/react";
import { MockLicenseResponse } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import LicensesSettingsPageView from "./LicensesSettingsPageView";

// Mock react-confetti to avoid canvas issues in tests
vi.mock("react-confetti", () => ({
	default: () => null,
}));

describe("LicensesSettingsPageView", () => {
	it("renders without crashing when userLimit values are undefined", async () => {
		render(
			<LicensesSettingsPageView
				showConfetti={false}
				isLoading={false}
				isRefreshing={false}
				userLimitActual={undefined}
				userLimitLimit={undefined}
				licenses={MockLicenseResponse}
				isRemovingLicense={false}
				removeLicense={() => {}}
				refreshEntitlements={() => {}}
				activeUsers={undefined}
				managedAgentFeature={undefined}
			/>,
		);

		await screen.findByText("Licenses");
	});
});
