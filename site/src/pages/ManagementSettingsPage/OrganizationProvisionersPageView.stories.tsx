import type { Meta, StoryObj } from "@storybook/react";
import { MockProvisioner, MockUserProvisioner } from "testHelpers/entities";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const meta: Meta<typeof OrganizationProvisionersPageView> = {
	title: "pages/OrganizationProvisionersPage",
	component: OrganizationProvisionersPageView,
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionersPageView>;

export const Provisioners: Story = {
	args: {
		provisioners: {
			builtin: [MockProvisioner, MockProvisioner],
			psk: [
				MockProvisioner,
				MockUserProvisioner,
				{
					...MockProvisioner,
					tags: {
						...MockProvisioner.tags,
						都市: "ユタ",
						きっぷ: "yes",
						ちいさい: "no",
					},
				},
			],
			keys: new Map([
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
						},
					],
				],
			]),
		},
	},
};
