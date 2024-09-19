import type { Meta, StoryObj } from "@storybook/react";
import {
	MockBuildInfo,
	MockProvisioner,
	MockProvisioner2,
	MockProvisionerWithTags,
	MockUserProvisioner,
} from "testHelpers/entities";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const meta: Meta<typeof OrganizationProvisionersPageView> = {
	title: "pages/OrganizationProvisionersPage",
	component: OrganizationProvisionersPageView,
	args: {
		buildInfo: MockBuildInfo,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionersPageView>;

export const Provisioners: Story = {
	args: {
		provisioners: {
			builtin: [MockProvisioner, MockProvisioner2],
			psk: [MockProvisioner, MockUserProvisioner, MockProvisionerWithTags],
			userAuth: [],
			keys: new Map([
				[
					"ベン",
					[
						MockProvisioner,
						{
							...MockProvisioner2,
							version: "2.0.0",
							api_version: "1.0",
							warnings: [{ code: "EUNKNOWN", message: "時代遅れです" }],
						},
					],
				],
				["ジャイデン", [MockProvisioner, MockProvisioner2]],
				[
					"ケイラ",
					[
						{
							...MockProvisioner,
							tags: {
								...MockProvisioner.tags,
								都市: "ユタ",
								きっぷ: "yes",
								ちいさい: "no",
							},
							warnings: [{ code: "EUNKNOWN", message: "日本語が話せません" }],
						},
					],
				],
			]),
		},
	},
};

export const Empty: Story = {
	args: {
		provisioners: {
			builtin: [],
			psk: [],
			userAuth: [],
			keys: new Map(),
		},
	},
};
