import { MockEntitlements, MockLicenseResponse } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, within } from "storybook/test";
import LicensesSettingsPage from "./LicensesSettingsPage";

const meta: Meta<typeof LicensesSettingsPage> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPage",
	component: LicensesSettingsPage,
	parameters: {
		queries: [
			{ key: ["licenses"], data: MockLicenseResponse },
			{ key: ["insights", "userStatusCounts"], data: { active: [] } },
		],
	},
};

export default meta;
type Story = StoryObj<typeof LicensesSettingsPage>;

const USER_STATUS_COUNTS_QUERY = {
	key: ["insights", "userStatusCounts"],
	data: { active: [] },
};

const withBaseQueries = ({
	entitlements = MockEntitlements,
	licenses = MockLicenseResponse,
}: {
	entitlements?: typeof MockEntitlements;
	licenses?: typeof MockLicenseResponse | unknown[];
}) => ({
	queries: [
		{ key: ["entitlements"], data: entitlements },
		{ key: ["licenses"], data: licenses },
		USER_STATUS_COUNTS_QUERY,
	],
});

const createEntitlements = ({
	userLimit,
	aiGovernanceUserLimit,
}: {
	userLimit: {
		enabled: boolean;
		entitlement: "entitled" | "not_entitled";
		actual: number;
		limit?: number;
	};
	aiGovernanceUserLimit?: {
		enabled: boolean;
		entitlement: "entitled" | "not_entitled" | "grace_period";
		actual: number;
		limit: number;
	};
}) => ({
	...MockEntitlements,
	has_license: true,
	features: {
		...MockEntitlements.features,
		user_limit: userLimit,
		ai_governance_user_limit:
			aiGovernanceUserLimit ??
			MockEntitlements.features.ai_governance_user_limit,
	},
});

const createLicense = ({
	id,
	uuid,
	featureSet,
	uploadedDaysAgo,
	expiresInDays,
	licenseExpiresInDays,
	nbfOffsetDays,
	aiGovernanceUserLimit,
	userLimit,
	addons,
}: {
	id: number;
	uuid: string;
	featureSet: "PREMIUM" | "enterprise";
	uploadedDaysAgo: number;
	expiresInDays: number;
	licenseExpiresInDays: number;
	nbfOffsetDays: number;
	aiGovernanceUserLimit: number;
	userLimit: number;
	addons?: string[];
}) => ({
	id,
	uploaded_at: String(dayjs().subtract(uploadedDaysAgo, "day").unix()),
	expires_at: String(dayjs().add(expiresInDays, "day").unix()),
	uuid,
	claims: {
		trial: false,
		all_features: true,
		feature_set: featureSet,
		version: 1,
		features: {
			ai_governance_user_limit: aiGovernanceUserLimit,
			user_limit: userLimit,
		},
		addons,
		license_expires: dayjs().add(licenseExpiresInDays, "day").unix(),
		nbf: dayjs().add(nbfOffsetDays, "day").unix(),
	},
});

export const WithoutUserLimitFeature: Story = {
	parameters: {
		...withBaseQueries({
			entitlements: {
				...MockEntitlements,
				features: {
					...MockEntitlements.features,
					user_limit: {
						enabled: false,
						entitlement: "not_entitled",
						actual: 4,
					},
				},
			},
		}),
	},
};

export const ShowsAddonUiForFutureLicenseBeforeNbf: Story = {
	parameters: {
		...withBaseQueries({
			entitlements: createEntitlements({
				userLimit: {
					enabled: true,
					entitlement: "entitled",
					actual: 3,
					limit: 10,
				},
				aiGovernanceUserLimit: {
					enabled: false,
					entitlement: "not_entitled",
					actual: 0,
					limit: 0,
				},
			}),
			licenses: [
				createLicense({
					id: 44,
					uuid: "future-premium-addon-license",
					featureSet: "PREMIUM",
					uploadedDaysAgo: 0,
					expiresInDays: 365,
					licenseExpiresInDays: 365,
					nbfOffsetDays: 7,
					aiGovernanceUserLimit: 100,
					userLimit: 10,
					addons: ["ai_governance"],
				}),
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/add-ons/i)).toBeInTheDocument();
		const aiGovernanceTitles = canvas.getAllByText(/^ai governance$/i);
		await expect(aiGovernanceTitles.length).toBeGreaterThan(0);
		await expect(canvas.getByText(/not started/i)).toBeInTheDocument();
		await expect(canvas.getByText(/valid from/i)).toBeInTheDocument();
	},
};
