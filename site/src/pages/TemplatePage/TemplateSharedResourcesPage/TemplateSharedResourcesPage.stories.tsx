import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import {
	foundationPermissionsKey,
	foundationsKey,
	templateFoundationKey,
} from "#/api/queries/foundations";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { MockFoundation } from "#/testHelpers/templateResources";
import { SharedResourceManagement } from "./TemplateSharedResourcesPage";

const mockUnboundFoundation = { ...MockFoundation, template_id: null };

const meta = {
	title: "pages/Templates/Foundation",
	component: SharedResourceManagement,
	decorators: [withDashboardProvider],
	args: { template: MockTemplate, canUpdateTemplate: true },
	parameters: {
		experiments: ["template-resources"],
		pixel: { matrix: pixelWithPhone },
		queries: [
			{
				key: foundationPermissionsKey(MockTemplate.organization_id),
				data: { canReadFoundation: true, canCreateFoundation: true },
			},
			{ key: templateFoundationKey(MockTemplate.id), data: MockFoundation },
		],
	},
} satisfies Meta<typeof SharedResourceManagement>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Attached: Story = {};
export const Unbound: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockTemplate.organization_id),
				data: { canReadFoundation: true, canCreateFoundation: true },
			},
			{ key: templateFoundationKey(MockTemplate.id), data: null },
			{
				key: foundationsKey(MockTemplate.organization_id),
				data: [mockUnboundFoundation],
			},
		],
	},
};
export const Loading: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockTemplate.organization_id),
				data: { canReadFoundation: true },
			},
		],
	},
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateFoundation").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const LoadFailure: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockTemplate.organization_id),
				data: { canReadFoundation: true },
			},
		],
	},
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateFoundation").mockRejectedValue(
			mockApiError({ message: "Association unavailable." }),
		);
		return () => mock.mockRestore();
	},
};
export const PermissionDenied: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockTemplate.organization_id),
				data: { canReadFoundation: false },
			},
		],
	},
};
