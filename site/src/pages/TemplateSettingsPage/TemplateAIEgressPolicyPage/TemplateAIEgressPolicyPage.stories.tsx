import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "react-query";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import {
	templateAIEgressPolicyKey,
	templateByNameKey,
} from "#/api/queries/templates";
import type { AIEgressPolicy } from "#/api/typesGenerated";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import {
	MockAIEgressPolicy,
	MockAIEgressPolicyEmpty,
	MockTemplate,
	mockApiError,
} from "#/testHelpers/entities";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import { TemplateSettingsLayout } from "../TemplateSettingsLayout";
import TemplateAIEgressPolicyPage from "./TemplateAIEgressPolicyPage";

let saveDeferred: Deferred<AIEgressPolicy> | undefined;
let refetchDeferred: Deferred<AIEgressPolicy> | undefined;

const withPolicyRefreshButton: Decorator = (Story) => {
	const queryClient = useQueryClient();
	return (
		<>
			<button
				type="button"
				className="m-4"
				onClick={() =>
					queryClient.invalidateQueries({
						queryKey: templateAIEgressPolicyKey(MockTemplate.id),
					})
				}
			>
				Refresh policy
			</button>
			<Story />
		</>
	);
};

const meta = {
	title: "pages/TemplateSettingsPage/TemplateAIEgressPolicyPage",
	component: TemplateSettingsLayout,
	decorators: [withToaster, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				path: "/templates/:template/settings/ai-egress-policy",
				pathParams: { template: MockTemplate.name },
			},
			routing: [
				{
					path: "/templates/:template/settings",
					useStoryElement: true,
					children: [
						{
							path: "ai-egress-policy",
							element: <TemplateAIEgressPolicyPage />,
						},
					],
				},
				{ path: "/templates/:template", element: <div>Template</div> },
			],
		}),
	},
} satisfies Meta<typeof TemplateSettingsLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const EmptyPolicyAddAndSave: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
			{
				key: templateAIEgressPolicyKey(MockTemplate.id),
				data: MockAIEgressPolicyEmpty,
			},
		],
	},
	beforeEach: () => {
		saveDeferred = createDeferred<AIEgressPolicy>();
		spyOn(API, "updateTemplateAIEgressPolicy").mockImplementation(
			() => saveDeferred?.promise ?? Promise.resolve(MockAIEgressPolicyEmpty),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		expect(await canvas.findByText("Revision 0")).toBeVisible();
		expect(canvas.getByText(/No explicit egress rules/)).toBeVisible();
		await user.click(canvas.getByRole("button", { name: "Add rule" }));

		const rule = canvas.getByRole("group", { name: "Rule 1" });
		await user.type(within(rule).getByLabelText("Host"), "example.com");
		await user.type(within(rule).getByLabelText("Ports"), "443");
		await user.click(canvas.getByRole("button", { name: "Save policy" }));

		await waitFor(() => {
			expect(API.updateTemplateAIEgressPolicy).toHaveBeenCalledWith(
				MockTemplate.id,
				{ rules: [{ host: "example.com", ports: [443] }] },
			);
		});
		expect(
			canvas.getByRole("button", { name: /Saving policy/ }),
		).toBeDisabled();

		const savedPolicy: AIEgressPolicy = {
			...MockAIEgressPolicyEmpty,
			revision: 1,
			rules: [{ host: "example.com", ports: [443] }],
			updated_at: "2026-08-17T13:00:00Z",
		};
		saveDeferred?.resolve(savedPolicy);

		expect(await canvas.findByText("Revision 1")).toBeVisible();
		expect(
			await body.findByText("AI egress policy updated successfully."),
		).toBeInTheDocument();
		expect(canvas.getByRole("button", { name: "Save policy" })).toBeDisabled();
	},
};

export const PopulatedPolicyEditsRemainOrdered: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
			{
				key: templateAIEgressPolicyKey(MockTemplate.id),
				data: MockAIEgressPolicy,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateTemplateAIEgressPolicy").mockImplementation(
			(_templateId, request) =>
				Promise.resolve({
					...MockAIEgressPolicy,
					revision: 8,
					rules: request.rules,
					updated_at: "2026-08-17T13:05:00Z",
				}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const firstRule = await canvas.findByRole("group", { name: "Rule 1" });
		const secondRule = canvas.getByRole("group", { name: "Rule 2" });

		const firstHost = within(firstRule).getByLabelText("Host");
		await user.clear(firstHost);
		await user.type(firstHost, "git.example.com");

		const secondPorts = within(secondRule).getByLabelText("Ports");
		await user.clear(secondPorts);
		await user.type(secondPorts, "443, 8443");
		await user.click(canvas.getByRole("button", { name: "Remove rule 3" }));
		await user.click(canvas.getByRole("button", { name: "Save policy" }));

		await waitFor(() => {
			expect(API.updateTemplateAIEgressPolicy).toHaveBeenCalledWith(
				MockTemplate.id,
				{
					rules: [
						{ host: "git.example.com", ports: [443] },
						{ host: "api.example.com", ports: [443, 8443] },
					],
				},
			);
		});
		expect(await canvas.findByText("Revision 8")).toBeVisible();
		expect(
			within(canvas.getByRole("group", { name: "Rule 2" })).getByLabelText(
				"Host",
			),
		).toHaveValue("api.example.com");
		expect(canvas.queryByText("registry.example.com")).not.toBeInTheDocument();
	},
};

export const ValidationBlocksMalformedRules: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
			{
				key: templateAIEgressPolicyKey(MockTemplate.id),
				data: MockAIEgressPolicyEmpty,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateTemplateAIEgressPolicy").mockResolvedValue(
			MockAIEgressPolicyEmpty,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(await canvas.findByRole("button", { name: "Add rule" }));
		const firstRule = canvas.getByRole("group", { name: "Rule 1" });
		const firstHost = within(firstRule).getByLabelText("Host");
		const firstPorts = within(firstRule).getByLabelText("Ports");
		await user.type(firstHost, "*.*.example.com");
		await user.type(firstPorts, "0");

		await user.click(canvas.getByRole("button", { name: "Add rule" }));
		const secondRule = canvas.getByRole("group", { name: "Rule 2" });
		const secondHost = within(secondRule).getByLabelText("Host");
		await user.type(secondHost, "https://example.com");

		expect(firstHost).toHaveAccessibleDescription(
			"Wildcard must be a single leading '*.' label.",
		);
		expect(firstPorts).toHaveAccessibleDescription(
			"Ports must be between 1 and 65535.",
		);
		expect(secondHost).toHaveAccessibleDescription(
			"Host must not contain a scheme, path, port, or user information.",
		);
		expect(canvas.getByRole("button", { name: "Save policy" })).toBeDisabled();
		expect(API.updateTemplateAIEgressPolicy).not.toHaveBeenCalled();
	},
};

export const SaveErrorPreservesEdits: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
			{
				key: templateAIEgressPolicyKey(MockTemplate.id),
				data: MockAIEgressPolicy,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateTemplateAIEgressPolicy").mockRejectedValue(
			mockApiError({
				message: "Failed to update AI egress policy.",
				detail: "The policy could not be stored.",
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const firstRule = await canvas.findByRole("group", { name: "Rule 1" });
		const firstHost = within(firstRule).getByLabelText("Host");
		await user.clear(firstHost);
		await user.type(firstHost, "edited.example.com");
		await user.click(canvas.getByRole("button", { name: "Save policy" }));

		expect(
			await canvas.findByText("Failed to update AI egress policy."),
		).toBeVisible();
		expect(firstHost).toHaveValue("edited.example.com");
		expect(canvas.getByText("Revision 7")).toBeVisible();
	},
};

export const ReadOnlyPermissions: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: false },
			},
			{
				key: templateAIEgressPolicyKey(MockTemplate.id),
				data: MockAIEgressPolicy,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText(/you do not have permission to update/i),
		).toBeVisible();
		const firstRule = canvas.getByRole("group", { name: "Rule 1" });
		expect(within(firstRule).getByLabelText("Host")).toBeDisabled();
		expect(within(firstRule).getByLabelText("Ports")).toBeDisabled();
		expect(canvas.getByRole("button", { name: "Add rule" })).toBeDisabled();
		expect(canvas.getByRole("button", { name: "Save policy" })).toBeDisabled();
		expect(canvas.getByDisplayValue("github.com")).toBeVisible();
	},
};

export const BackgroundRefetchPreservesEdits: Story = {
	decorators: [withPolicyRefreshButton],
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
			{
				key: templateAIEgressPolicyKey(MockTemplate.id),
				data: MockAIEgressPolicy,
			},
		],
	},
	beforeEach: () => {
		refetchDeferred = createDeferred<AIEgressPolicy>();
		spyOn(API, "getTemplateAIEgressPolicy").mockImplementation(
			() => refetchDeferred?.promise ?? Promise.resolve(MockAIEgressPolicy),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const firstRule = await canvas.findByRole("group", { name: "Rule 1" });
		const firstHost = within(firstRule).getByLabelText("Host");
		await user.clear(firstHost);
		await user.type(firstHost, "in-progress.example.com");
		await user.click(canvas.getByRole("button", { name: "Refresh policy" }));

		expect(await canvas.findByRole("status")).toHaveTextContent(
			"Refreshing policy",
		);
		expect(firstHost).toHaveValue("in-progress.example.com");

		refetchDeferred?.resolve({
			...MockAIEgressPolicy,
			revision: 8,
			rules: [{ host: "server.example.com", ports: [443] }],
		});
		await waitFor(() => {
			expect(canvas.queryByText("Refreshing policy")).not.toBeInTheDocument();
		});
		expect(firstHost).toHaveValue("in-progress.example.com");
	},
};

export const LoadingPolicy: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplateAIEgressPolicy").mockImplementation(
			() => new Promise<AIEgressPolicy>(() => undefined),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByLabelText("Loading AI egress policy"),
		).toBeVisible();
	},
};

export const LoadError: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplateAIEgressPolicy").mockRejectedValue(
			mockApiError({ message: "Failed to load AI egress policy." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load AI egress policy."),
		).toBeVisible();
	},
};
