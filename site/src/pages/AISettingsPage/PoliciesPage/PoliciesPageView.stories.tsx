import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import type {
	AIGatewayGuardrail,
	AIGatewayPipeline,
	AIGatewayPolicy,
} from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import PoliciesPageView from "./PoliciesPageView";

const policyVersionId = "00000000-0000-0000-0000-0000000000a1";
const policyVersionId2 = "00000000-0000-0000-0000-0000000000a2";

const mockPolicy: AIGatewayPolicy = {
	id: "00000000-0000-0000-0000-000000000001",
	name: "model-allowlist",
	display_name: "Model allowlist",
	kind: "decide",
	active_version_id: policyVersionId2,
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	versions: [
		{
			id: policyVersionId2,
			policy_id: "00000000-0000-0000-0000-000000000001",
			version_number: 2,
			rego: 'default verdict := "ALLOW"',
			input_schema_version: 1,
			output_schema_version: 1,
			description: "",
			created_at: "2024-01-02T00:00:00Z",
		},
		{
			id: policyVersionId,
			policy_id: "00000000-0000-0000-0000-000000000001",
			version_number: 1,
			rego: 'default verdict := "BLOCK"',
			input_schema_version: 1,
			output_schema_version: 1,
			description: "",
			created_at: "2024-01-01T00:00:00Z",
		},
	],
};

const liveVersionId = "00000000-0000-0000-0000-0000000000b1";
const stagedVersionId = "00000000-0000-0000-0000-0000000000b2";

const guardrailVersionId = "00000000-0000-0000-0000-0000000000c1";

const mockGuardrail: AIGatewayGuardrail = {
	id: "00000000-0000-0000-0000-0000000000c0",
	name: "presidio-pii",
	display_name: "Presidio PII",
	adapter_type: "presidio",
	active_version_id: guardrailVersionId,
	enabled: true,
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	versions: [
		{
			id: guardrailVersionId,
			guardrail_id: "00000000-0000-0000-0000-0000000000c0",
			version_number: 1,
			config: {},
			has_credential: false,
			description: "",
			created_at: "2024-01-01T00:00:00Z",
		},
	],
};

// A pipeline whose live (active) version still pins the old policy version and
// has no guardrail, while a newer unpromoted version sits on the tip with a
// staged guardrail: minted-but-unpromoted drift. The edit surface must base off
// the tip so the staged guardrail is preserved, not the active version.
const driftedPipeline: AIGatewayPipeline = {
	id: "00000000-0000-0000-0000-000000000010",
	provider_id: MockAIProviderOpenAI.id,
	enabled: true,
	active_version_id: liveVersionId,
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	active_version: {
		id: liveVersionId,
		pipeline_id: "00000000-0000-0000-0000-000000000010",
		version_number: 1,
		created_at: "2024-01-01T00:00:00Z",
		policies: [
			{
				policy_version_id: policyVersionId,
				hook: "pre_req",
				kind: "decide",
				fail_mode: "fail_closed",
				enabled: true,
			},
		],
		guardrails: [],
	},
	latest_version_id: stagedVersionId,
	latest_version_number: 2,
	latest_version: {
		id: stagedVersionId,
		pipeline_id: "00000000-0000-0000-0000-000000000010",
		version_number: 2,
		created_at: "2024-01-02T00:00:00Z",
		policies: [
			{
				policy_version_id: policyVersionId,
				hook: "pre_req",
				kind: "decide",
				fail_mode: "fail_closed",
				enabled: true,
			},
		],
		guardrails: [
			{
				guardrail_version_id: guardrailVersionId,
				hook: "pre_req",
				mode: "advisory",
				fail_mode: "fail_closed",
				network_timeout_ms: 2000,
				enabled: true,
			},
		],
	},
};

// A pipeline with no pending changes: live version equals the tip.
const promotedPipeline: AIGatewayPipeline = {
	id: "00000000-0000-0000-0000-000000000020",
	provider_id: MockAIProviderAnthropic.id,
	enabled: true,
	active_version_id: liveVersionId,
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	active_version: {
		id: liveVersionId,
		pipeline_id: "00000000-0000-0000-0000-000000000020",
		version_number: 1,
		created_at: "2024-01-01T00:00:00Z",
		policies: [
			{
				policy_version_id: policyVersionId2,
				hook: "pre_req",
				kind: "decide",
				fail_mode: "fail_closed",
				enabled: true,
			},
		],
		guardrails: [],
	},
	latest_version_id: liveVersionId,
	latest_version_number: 1,
};

const meta: Meta<typeof PoliciesPageView> = {
	title: "pages/AISettingsPage/PoliciesPageView",
	component: PoliciesPageView,
	args: {
		policies: [mockPolicy],
		pipelines: [driftedPipeline, promotedPipeline],
		providers: [MockAIProviderOpenAI, MockAIProviderAnthropic],
		guardrails: [mockGuardrail],
		isLoading: false,
		error: undefined,
		onCreatePolicy: fn(),
		isCreating: false,
		createError: undefined,
		onDeletePolicy: fn(),
		deletePolicyError: undefined,
		onEditPolicy: fn(),
		isEditing: false,
		editError: undefined,
		onRevertPolicy: fn(),
		isReverting: false,
		revertError: undefined,
		onCreatePipeline: fn(),
		isCreatingPipeline: false,
		createPipelineError: undefined,
		onDeletePipeline: fn(),
		deletePipelineError: undefined,
		onEditPipeline: fn(),
		isEditingPipeline: false,
		editPipelineError: undefined,
		onTogglePipeline: fn(),
		onToggleMember: fn(),
		onPromotePipeline: fn(),
		isPromoting: false,
		promoteError: undefined,
	},
};

export default meta;
type Story = StoryObj<typeof PoliciesPageView>;

export const Default: Story = {};

// The drifted pipeline shows an "Unpromoted" badge; the promoted one does not.
export const ShowsUnpromotedDrift: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Unpromoted v2")).toBeVisible();
	},
};

// Promoting takes the pipeline's tip version live.
export const PromoteUnpromotedChanges: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const promote = await canvas.findByRole("button", { name: "Promote v2" });
		await userEvent.click(promote);
		await expect(args.onPromotePipeline).toHaveBeenCalledWith(
			driftedPipeline.id,
			stagedVersionId,
			expect.any(Function),
		);
	},
};

// Regression: editing a pipeline with an unpromoted draft must base the new
// version on the tip, not the active version, so a guardrail staged in the
// draft is not dropped. The edit dialog opens pre-populated from the tip and
// saving forwards the staged guardrail.
export const EditPreservesStagedGuardrail: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const editButtons = await canvas.findAllByRole("button", {
			name: "Edit policies",
		});
		// The drifted pipeline (with the staged guardrail) renders first.
		await userEvent.click(editButtons[0]);

		// The dialog portals to document.body, so query via screen.
		const dialog = within(await screen.findByRole("dialog"));
		// The dialog pre-populates the guardrail editor from the tip version.
		const save = await dialog.findByRole("button", {
			name: "Save new version",
		});
		await userEvent.click(save);

		// The minted version request must still carry the staged guardrail.
		await expect(args.onEditPipeline).toHaveBeenCalledWith(
			driftedPipeline.id,
			expect.anything(),
			expect.arrayContaining([
				expect.objectContaining({ guardrail_version_id: guardrailVersionId }),
			]),
			expect.any(Function),
		);
	},
};

// Enabling/disabling a member is a live, in-place toggle: it calls
// onToggleMember (PATCH the active version's membership) and does NOT mint a new
// pipeline version via onEditPipeline.
export const ToggleMemberIsInPlace: Story = {
	args: { pipelines: [promotedPipeline] },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		// Expand the pipeline row to reveal its attached members.
		await userEvent.click(
			await canvas.findByRole("button", { name: /Show policies/ }),
		);
		// Disable buttons: [pipeline-level, the policy member]. The member's is
		// rendered last (inside the expanded row).
		const disables = await canvas.findAllByRole("button", { name: "Disable" });
		await userEvent.click(disables[disables.length - 1]);

		await expect(args.onToggleMember).toHaveBeenCalledWith(
			promotedPipeline.id,
			{
				policy_version_id: policyVersionId2,
				hook: "pre_req",
				enabled: false,
			},
		);
		// Crucially, no new pipeline version is minted for a pause toggle.
		await expect(args.onEditPipeline).not.toHaveBeenCalled();
	},
};
