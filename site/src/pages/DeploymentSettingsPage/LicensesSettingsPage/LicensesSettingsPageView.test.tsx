import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CONTACT_SALES_LINK } from "#/modules/licenses/trialLicense";
import { mockApiError } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import LicensesSettingsPageView from "./LicensesSettingsPageView";

// jsdom has no canvas, which react-confetti requires.
vi.mock("react-confetti", () => ({ default: () => null }));

const renderView = (
	props: Partial<React.ComponentProps<typeof LicensesSettingsPageView>>,
) =>
	render(
		<LicensesSettingsPageView
			showConfetti={false}
			isLoading={false}
			hasUserLimitEntitlementData={false}
			licenses={[]}
			canViewLicenses
			isRemovingLicense={false}
			isRefreshing={false}
			removeLicense={vi.fn()}
			refreshEntitlements={vi.fn()}
			activeUsers={undefined}
			{...props}
		/>,
	);

describe("LicensesSettingsPageView", () => {
	it("links to add a license, contact sales, and request a trial when there are no licenses", async () => {
		renderView({ licenses: [] });

		await screen.findByText("No licenses yet");
		for (const link of screen.getAllByRole("link", {
			name: /add a license/i,
		})) {
			expect(link).toHaveAttribute("href", "/deployment/licenses/add");
		}
		expect(
			screen.getByRole("link", { name: /contact sales/i }),
		).toHaveAttribute("href", CONTACT_SALES_LINK);
		expect(
			screen.getByRole("link", { name: /request a trial license/i }),
		).toHaveAttribute("href", "/deployment/premium");
	});

	it("shows a retryable error instead of the empty state when licenses fail to load", async () => {
		const user = userEvent.setup();
		const onRetryLicenses = vi.fn();
		renderView({
			licenses: undefined,
			licensesError: mockApiError({
				message: "Internal error fetching licenses.",
			}),
			onRetryLicenses,
		});

		await screen.findByText("Internal error fetching licenses.");
		expect(screen.queryByText("No licenses yet")).not.toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: /retry/i }));
		expect(onRetryLicenses).toHaveBeenCalledTimes(1);
	});
});
