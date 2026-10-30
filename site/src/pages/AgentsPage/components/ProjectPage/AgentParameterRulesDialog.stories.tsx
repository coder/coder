import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import { richParametersKey } from "#/api/queries/templates";
import {
	MockRegionParameter,
	MockRepoParameter,
	mockApiError,
} from "#/testHelpers/entities";
import { AgentParameterRulesDialog } from "./AgentParameterRulesDialog";

const mockRegionRule = {
	template_id: "template-security",
	parameter_name: "region",
	allowed_values: ["eu-west", "us-east"],
};

const mockRepoRule = {
	template_id: "template-security",
	parameter_name: "repo",
	allowed_values: ["coder/coder", "coder/website"],
};

const meta: Meta<typeof AgentParameterRulesDialog> = {
	title: "pages/AgentsPage/ProjectPage/AgentParameterRulesDialog",
	component: AgentParameterRulesDialog,
	args: {
		rules: [mockRegionRule, mockRepoRule],
		template: {
			id: "template-security",
			label: "Security scanner",
			activeVersionId: "version-security",
		},
		isSaving: false,
		error: undefined,
		onClose: fn(),
		onSave: fn(),
	},
	parameters: {
		queries: [
			{
				key: richParametersKey("version-security"),
				data: [MockRegionParameter, MockRepoParameter],
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof AgentParameterRulesDialog>;

export const Restricted: Story = {};

export const Unrestricted: Story = { args: { rules: [] } };

export const RemovedParameter: Story = {
	args: {
		rules: [
			{
				template_id: "template-security",
				parameter_name: "instance_type",
				allowed_values: ["small"],
			},
		],
	},
};

export const EmptyAllowedValues: Story = {
	args: { rules: [{ ...mockRepoRule, allowed_values: [] }] },
};

export const NoParameters: Story = {
	args: { rules: [] },
	parameters: {
		queries: [{ key: richParametersKey("version-security"), data: [] }],
	},
};

export const LoadingParameters: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getTemplateVersionRichParameters").mockReturnValue(
			new Promise(() => undefined),
		);
	},
};

export const ParametersError: Story = {
	args: { rules: [] },
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getTemplateVersionRichParameters").mockRejectedValue(
			mockApiError({ message: "Failed to load parameters." }),
		);
	},
};

export const Saving: Story = { args: { isSaving: true } };

export const SaveError: Story = {
	args: {
		error: mockApiError({ message: "Failed to save template parameters." }),
	},
};
