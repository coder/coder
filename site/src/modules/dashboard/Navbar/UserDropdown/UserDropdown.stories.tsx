import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import dayjs, { type Dayjs } from "dayjs";
import { useContext } from "react";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API, type GetLicensesResponse } from "#/api/api";
import { licensesKey } from "#/api/queries/licenses";
import { meAISpendKey } from "#/api/queries/users";
import type {
	AIBudgetLimit,
	Entitlements,
	FeatureName,
	UserAISpendStatus,
} from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockBuildInfo,
	MockLicenseResponse,
	MockMultiOrgExceededUserAISpendStatus,
	MockMultiOrgUserAISpendStatus,
	MockUserAISpendStatus,
	MockUserOrganizationAISpend,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { UserDropdown } from "./UserDropdown";

// These single-organization variants keep the top-level spend and their one
// organizations entry in agreement, as the API returns them.

const mockWarningAISpend: UserAISpendStatus = {
	...MockUserAISpendStatus,
	current_spend_micros: 1_080_000_000,
	organizations: [
		{ ...MockUserOrganizationAISpend, current_spend_micros: 1_080_000_000 },
	],
};

const mockExceededAISpend: UserAISpendStatus = {
	...MockUserAISpendStatus,
	current_spend_micros: 1_500_000_000,
	organizations: [
		{ ...MockUserOrganizationAISpend, current_spend_micros: 1_500_000_000 },
	],
};

const mockAtLimitAISpend: UserAISpendStatus = {
	...MockUserAISpendStatus,
	current_spend_micros: 1_200_000_000,
	organizations: [
		{ ...MockUserOrganizationAISpend, current_spend_micros: 1_200_000_000 },
	],
};

const mockUnlimitedAISpend: UserAISpendStatus = {
	...MockUserAISpendStatus,
	effective_budget: null,
	organizations: [{ ...MockUserOrganizationAISpend, effective_budget: null }],
};

const mockZeroSpendAISpend: UserAISpendStatus = {
	...MockUserAISpendStatus,
	current_spend_micros: 0,
	organizations: [{ ...MockUserOrganizationAISpend, current_spend_micros: 0 }],
};

const mockZeroLimitBudget: AIBudgetLimit = {
	spend_limit_micros: 0,
	limit_source: "group",
};

const mockZeroLimitAISpend: UserAISpendStatus = {
	...MockUserAISpendStatus,
	current_spend_micros: 0,
	effective_budget: mockZeroLimitBudget,
	organizations: [
		{
			...MockUserOrganizationAISpend,
			current_spend_micros: 0,
			effective_budget: mockZeroLimitBudget,
		},
	],
};

// The shown organization reports a negative spend, so the menu keeps the
// selector and says the spend is unavailable.
const mockMultiOrgInvalidAISpend: UserAISpendStatus = {
	...MockMultiOrgUserAISpendStatus,
	current_spend_micros: -1,
	organizations: [
		{ ...MockUserOrganizationAISpend, current_spend_micros: -1 },
		...MockMultiOrgUserAISpendStatus.organizations.slice(1),
	],
};

const spendPeriodLabel = "Approximate AI spend June 1 - July 1, 2026";

const aiCostControl: { features: FeatureName[] } = {
	features: ["aibridge"],
};

const meta: Meta<typeof UserDropdown> = {
	title: "modules/dashboard/UserDropdown",
	component: UserDropdown,
	args: {
		user: MockUserOwner,
		buildInfo: MockBuildInfo,
		canViewLicenses: false,
		supportLinks: [
			{ icon: "docs", name: "Documentation", target: "" },
			{ icon: "bug", name: "Report a bug", target: "" },
			{ icon: "chat", name: "Join the Coder Discord", target: "" },
			{ icon: "star", name: "Star the Repo", target: "" },
			{ icon: "/icon/aws.svg", name: "Amazon Web Services", target: "" },
		],
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof UserDropdown>;

const openDropdown = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button"));
	return within(
		await within(canvasElement.ownerDocument.body).findByRole("menu"),
	);
};

// Overrides platform detection so the Coder Desktop gating can be exercised in
// a story. Returns a cleanup that restores the spied getters.
const mockPlatform = (platform: string, maxTouchPoints = 0) => {
	const platformSpy = spyOn(navigator, "platform", "get").mockReturnValue(
		platform,
	);
	const touchSpy = spyOn(navigator, "maxTouchPoints", "get").mockReturnValue(
		maxTouchPoints,
	);
	return () => {
		platformSpy.mockRestore();
		touchSpy.mockRestore();
	};
};

const Example: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend without the aibridge feature", async () => {
			await openDropdown(canvasElement);
			expect(screen.queryByText(/AI spend/i)).not.toBeInTheDocument();
		});
	},
};

export const WithAISpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows AI spend", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$819 / $1,200 USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.getByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).toHaveAttribute("aria-valuenow", "68");
		});
	},
};

export const AISpendWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockWarningAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the warning marker near the limit", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$1,080 / $1,200 USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.getByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).toHaveAttribute("aria-valuenow", "90");
		});
	},
};

export const AISpendAtLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockAtLimitAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("marks spend at the limit as exceeded", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$1,200 / $1,200 USD"),
			);
			expect(
				screen.getByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).toHaveAttribute("aria-valuenow", "100");
		});
	},
};

export const AISpendExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockExceededAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the exceeded marker at the limit", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$1,500 / $1,200 USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.getByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).toHaveAttribute("aria-valuenow", "100");
		});
	},
};

export const AISpendUnlimited: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockUnlimitedAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows unlimited spend without a bar", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$819 / Unlimited USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.queryByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).not.toBeInTheDocument();
		});
	},
};

export const AISpendZeroSpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockZeroSpendAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows zero spend with an empty bar", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$0 / $1,200 USD"),
			);
			expect(
				screen.getByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).toHaveAttribute("aria-valuenow", "0");
		});
	},
};

export const AISpendZeroLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockZeroLimitAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows a zero limit as exceeded", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$0 / $0 USD"),
			);
			expect(document.body).toHaveTextContent("Limit exceeded");
			expect(
				screen.getByRole("progressbar", {
					hidden: true,
					name: "AI spend usage",
				}),
			).toHaveAttribute("aria-valuenow", "0");
		});
	},
};

export const MultiOrgAISpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: MockMultiOrgUserAISpendStatus }],
	},
	play: async ({ canvasElement }) => {
		await openDropdown(canvasElement);
	},
};

export const MultiOrgAISpendPicker: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: MockMultiOrgUserAISpendStatus }],
	},
	play: async ({ canvasElement }) => {
		const menu = await openDropdown(canvasElement);
		await userEvent.click(
			await menu.findByRole("menuitem", { name: /^Organization:/ }),
		);
	},
};

export const MultiOrgAISpendExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{ key: meAISpendKey, data: MockMultiOrgExceededUserAISpendStatus },
		],
	},
	play: async ({ canvasElement }) => {
		await openDropdown(canvasElement);
	},
};

export const MultiOrgAISpendUnavailable: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: mockMultiOrgInvalidAISpend }],
	},
	play: async ({ canvasElement }) => {
		await openDropdown(canvasElement);
	},
};

// Dropdown closed to isolate the avatar and its severity badge, which
// indicates AI spend limit severity.

export const AvatarBorderDisabled: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
};

export const AvatarBorderNormal: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows no severity indicator for normal spend", async () => {
			const canvas = within(canvasElement);
			expect(
				canvas.getByRole("button", { name: /User menu$/ }),
			).toBeInTheDocument();
		});
	},
};

export const AvatarBorderWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockWarningAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("labels the trigger with the warning state", async () => {
			const canvas = within(canvasElement);
			await canvas.findByRole("button", {
				name: /User menu\. AI spend is nearing its limit$/,
			});
		});
	},
};

export const AvatarBorderExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: mockExceededAISpend,
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("labels the trigger with the exceeded state", async () => {
			const canvas = within(canvasElement);
			await canvas.findByRole("button", {
				name: /User menu\. AI spend limit exceeded$/,
			});
		});
	},
};

export const AISpendHiddenOnInvalidData: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: { ...MockUserAISpendStatus, current_spend_micros: -1 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend on invalid data", async () => {
			await openDropdown(canvasElement);
			expect(document.body).not.toHaveTextContent(spendPeriodLabel);
		});
	},
};

export const AISpendHiddenOnNegativeLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: {
					...MockUserAISpendStatus,
					effective_budget: { spend_limit_micros: -1, limit_source: "group" },
				},
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend on a negative limit", async () => {
			await openDropdown(canvasElement);
			expect(document.body).not.toHaveTextContent(spendPeriodLabel);
		});
	},
};

export const InstallCoderDesktopMacOS: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	beforeEach: () => mockPlatform("MacIntel"),
	play: async ({ canvasElement, step }) => {
		await step(
			"links Install Coder Desktop to the docs alongside Install CLI",
			async () => {
				const menu = await openDropdown(canvasElement);
				expect(
					menu.getByRole("menuitem", { name: "Install Coder Desktop" }),
				).toHaveAttribute("href", "https://coder.com/docs/user-guides/desktop");
				expect(
					menu.getByRole("menuitem", { name: "Install CLI" }),
				).toBeInTheDocument();
			},
		);
	},
};

export const InstallCoderDesktopWindows: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	beforeEach: () => mockPlatform("Win32"),
	play: async ({ canvasElement, step }) => {
		await step("shows Install Coder Desktop on Windows", async () => {
			const menu = await openDropdown(canvasElement);
			expect(
				menu.getByRole("menuitem", { name: "Install Coder Desktop" }),
			).toBeInTheDocument();
		});
	},
};

export const InstallCoderDesktopHiddenOnLinux: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	beforeEach: () => mockPlatform("Linux x86_64"),
	play: async ({ canvasElement, step }) => {
		await step(
			"hides Install Coder Desktop but keeps Install CLI",
			async () => {
				const menu = await openDropdown(canvasElement);
				expect(
					menu.queryByRole("menuitem", { name: "Install Coder Desktop" }),
				).not.toBeInTheDocument();
				expect(
					menu.getByRole("menuitem", { name: "Install CLI" }),
				).toBeInTheDocument();
			},
		);
	},
};

export const InstallCoderDesktopHiddenOniPadOS: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: MockUserAISpendStatus }],
	},
	// iPadOS 13+ reports "MacIntel" but exposes a touchscreen.
	beforeEach: () => mockPlatform("MacIntel", 5),
	play: async ({ canvasElement, step }) => {
		await step("hides Install Coder Desktop on iPadOS", async () => {
			const menu = await openDropdown(canvasElement);
			expect(
				menu.queryByRole("menuitem", { name: "Install Coder Desktop" }),
			).not.toBeInTheDocument();
		});
	},
};

export const CodernautsDisabled: Story = {
	args: {
		codernautsEnabled: false,
	},
	play: async ({ canvasElement, step }) => {
		await step("hides the Codernauts link", async () => {
			await openDropdown(canvasElement);
			expect(
				screen.queryByRole("menuitem", { name: "Codernauts" }),
			).not.toBeInTheDocument();
		});
	},
};

export { Example as UserDropdown };

// Trial CTA and countdown.

const trialEntitlements = { has_license: true, trial: true };
const licensedEntitlements = { has_license: true, trial: false };
const unlicensedEntitlements = { has_license: false, trial: false };

/**
 * Overrides the entitlements withDashboardProvider supplies. It derives
 * has_license from the feature list and cannot express `trial` at all, so trial
 * states re-provide the context instead. Story decorators render inside meta
 * decorators, so this wins over the provider above it.
 */
const withEntitlements =
	(overrides: Partial<Entitlements>): Decorator =>
	(Story) => {
		const dashboard = useContext(DashboardContext);
		if (!dashboard) {
			throw new Error(
				"withEntitlements must be composed inside withDashboardProvider",
			);
		}

		return (
			<DashboardContext.Provider
				value={{
					...dashboard,
					entitlements: { ...dashboard.entitlements, ...overrides },
				}}
			>
				<Story />
			</DashboardContext.Provider>
		);
	};

// Offsets carry an extra hour so the truncating day math cannot land a story on
// the boundary and flip its expected label between runs.
const inDays = (days: number): Dayjs => dayjs().add(days, "day").add(1, "hour");

const [baseLicense] = MockLicenseResponse;

const trialLicenses = (expiresAt: Dayjs): GetLicensesResponse[] => [
	{
		...baseLicense,
		uuid: "trial-1",
		expires_at: `${expiresAt.unix()}`,
		claims: {
			...baseLicense.claims,
			trial: true,
			license_expires: expiresAt.unix(),
		},
	},
];

// Asserts the menu is intact regardless of what the licenses request did. UserDropdown
// renders on every page, so a license failure must never degrade the navbar.
const expectMenuUsable = async (canvasElement: HTMLElement) => {
	const menu = await openDropdown(canvasElement);
	expect(menu.getByRole("menuitem", { name: "Account" })).toBeInTheDocument();
	expect(menu.getByRole("menuitem", { name: "Sign Out" })).toBeInTheDocument();
	return menu;
};

const expectNoTrialEntry = (menu: ReturnType<typeof within>) => {
	expect(
		menu.queryByRole("menuitem", { name: /trial/i }),
	).not.toBeInTheDocument();
	expect(
		menu.queryByRole("menuitem", { name: /premium/i }),
	).not.toBeInTheDocument();
	expect(
		menu.queryByRole("menuitem", { name: "Expires today" }),
	).not.toBeInTheDocument();
};

export const TrialCtaStart: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(unlicensedEntitlements)],
	play: async ({ canvasElement, step }) => {
		await step("offers the trial and links to the premium page", async () => {
			const menu = await expectMenuUsable(canvasElement);
			expect(
				menu.getByRole("menuitem", { name: "Try premium for 30 days" }),
			).toHaveAttribute("href", "/deployment/premium");
		});
	},
};

export const TrialCountdown: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(trialEntitlements)],
	parameters: {
		queries: [{ key: licensesKey, data: trialLicenses(inDays(3)) }],
	},
	play: async ({ canvasElement, step }) => {
		await step("counts down the remaining trial days", async () => {
			const menu = await expectMenuUsable(canvasElement);
			expect(
				menu.getByRole("menuitem", { name: "3 days left in trial" }),
			).toHaveAttribute("href", "/deployment/premium");
		});
	},
};

export const TrialCountdownFinalDay: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(trialEntitlements)],
	parameters: {
		queries: [
			{ key: licensesKey, data: trialLicenses(dayjs().add(18, "hour")) },
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("keeps the entry visible on the final day", async () => {
			const menu = await expectMenuUsable(canvasElement);
			expect(
				menu.getByRole("menuitem", { name: "Trial expires today" }),
			).toBeInTheDocument();
		});
	},
};

export const TrialCtaHiddenInGracePeriod: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(trialEntitlements)],
	parameters: {
		queries: [{ key: licensesKey, data: trialLicenses(inDays(-2)) }],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides the countdown once the trial has expired", async () => {
			expectNoTrialEntry(await expectMenuUsable(canvasElement));
		});
	},
};

export const TrialCtaHiddenWithLicense: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(licensedEntitlements)],
	play: async ({ canvasElement, step }) => {
		await step("hides the offer once a real license is in place", async () => {
			expectNoTrialEntry(await expectMenuUsable(canvasElement));
		});
	},
};

export const TrialCtaHiddenForNonAdmin: Story = {
	args: { canViewLicenses: false },
	decorators: [withEntitlements(trialEntitlements)],
	parameters: {
		// permission gate holds even when the licenses page has warm cache entry.
		queries: [{ key: licensesKey, data: trialLicenses(inDays(3)) }],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides every trial state from non-admins", async () => {
			expectNoTrialEntry(await expectMenuUsable(canvasElement));
		});
	},
};

export const NavbarSurvivesLicenseFetchError: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(trialEntitlements)],
	beforeEach: () => {
		const spy = spyOn(API, "getLicenses").mockRejectedValue(
			new Error("licenses unavailable"),
		);
		return () => spy.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		await step("keeps the menu usable when licenses fail", async () => {
			expectNoTrialEntry(await expectMenuUsable(canvasElement));
		});
	},
};

export const NavbarSurvivesPendingLicenseFetch: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(trialEntitlements)],
	beforeEach: () => {
		const spy = spyOn(API, "getLicenses").mockImplementation(
			() => new Promise<GetLicensesResponse[]>(() => {}),
		);
		return () => spy.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		await step("keeps the menu usable while licenses load", async () => {
			expectNoTrialEntry(await expectMenuUsable(canvasElement));
		});
	},
};

export const NavbarSurvivesEmptyLicenseList: Story = {
	args: { canViewLicenses: true },
	decorators: [withEntitlements(trialEntitlements)],
	parameters: {
		queries: [{ key: licensesKey, data: [] }],
	},
	play: async ({ canvasElement, step }) => {
		await step(
			"keeps the menu usable with no trial license found",
			async () => {
				expectNoTrialEntry(await expectMenuUsable(canvasElement));
			},
		);
	},
};
