import "#/testHelpers/monaco";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HCLEditor } from "#/components/HCLEditor/HCLEditor";
import { MockTemplate } from "#/testHelpers/entities";
import {
	MockNetworkPolicy,
	MockNetworkPolicyRevision,
	MockNetworkPolicyTarget,
	MockNetworkPolicyTemplate,
} from "#/testHelpers/networkPolicies";
import { CreateNetworkPolicyForm } from "./CreateNetworkPolicyForm";
import { NetworkPolicyAttachments } from "./NetworkPolicyAttachments";
import { NetworkPolicyEditor } from "./NetworkPolicyEditor";
import { NetworkPolicyPublications } from "./NetworkPolicyPublications";
import { NetworkPolicyTargetForm } from "./NetworkPolicyTargetForm";

it("reports cleared HCL and blur when tabbing out of the editor", async () => {
	const onChange = vi.fn();
	const onBlur = vi.fn();
	const user = userEvent.setup();
	render(
		<HCLEditor
			label="Policy HCL"
			readOnly={false}
			name="hcl"
			value={MockNetworkPolicyRevision.hcl}
			onChange={onChange}
			onBlur={onBlur}
		/>,
	);
	await user.clear(screen.getByRole("textbox", { name: "Policy HCL" }));
	expect(onChange).toHaveBeenCalledWith("");
	expect(onBlur).not.toHaveBeenCalled();
	await user.tab();
	expect(onBlur).toHaveBeenCalledTimes(1);
});

it("creates a policy with real selected template IDs and edited HCL", async () => {
	const onSubmit = vi.fn();
	const user = userEvent.setup();
	render(
		<CreateNetworkPolicyForm
			templates={[MockTemplate, MockNetworkPolicyTemplate]}
			isPending={false}
			error={undefined}
			onSubmit={onSubmit}
		/>,
	);
	await user.type(
		screen.getByRole("textbox", { name: /^Name/ }),
		"team-policy",
	);
	await user.type(
		screen.getByRole("textbox", { name: "Description" }),
		"Team egress",
	);
	await user.click(screen.getByRole("combobox", { name: "Templates" }));
	await user.click(
		await screen.findByRole("option", {
			name: MockTemplate.display_name || MockTemplate.name,
		}),
	);
	await user.click(screen.getByRole("combobox", { name: "Templates" }));
	await user.click(
		await screen.findByRole("option", {
			name:
				MockNetworkPolicyTemplate.display_name ||
				MockNetworkPolicyTemplate.name,
		}),
	);
	await user.clear(screen.getByRole("textbox", { name: "Policy HCL" }));
	await user.type(
		screen.getByRole("textbox", { name: "Policy HCL" }),
		"custom policy HCL",
	);
	await user.tab();
	await user.keyboard("{Enter}");
	expect(onSubmit).toHaveBeenCalledWith({
		name: "team-policy",
		description: "Team egress",
		hcl: "custom policy HCL",
		template_ids: [MockTemplate.id, MockNetworkPolicyTemplate.id],
	});
});

it("preserves HCL and its concurrency generation across background refetches", async () => {
	const onSave = vi.fn();
	const user = userEvent.setup();
	const props = {
		policy: MockNetworkPolicy,
		selectedRevisionId: MockNetworkPolicyRevision.id,
		onSelectRevision: vi.fn(),
		isPending: false,
		readOnly: false,
		error: undefined,
		onSave,
	};
	const { rerender } = render(<NetworkPolicyEditor {...props} />);
	await user.clear(screen.getByRole("textbox", { name: "Policy HCL" }));
	await user.type(
		screen.getByRole("textbox", { name: "Policy HCL" }),
		"my unsaved edit",
	);
	rerender(
		<NetworkPolicyEditor
			{...props}
			policy={{
				...MockNetworkPolicy,
				generation: 99,
				revisions: [{ ...MockNetworkPolicyRevision, hcl: "server HCL" }],
			}}
		/>,
	);
	await user.click(screen.getByRole("button", { name: "Save new version" }));
	expect(onSave).toHaveBeenCalledWith(
		{
			hcl: "my unsaved edit",
			expected_generation: MockNetworkPolicy.generation,
		},
		expect.any(Function),
	);
});

it("does not change pending policy HCL and restores editing after saving", async () => {
	const user = userEvent.setup();
	const onSubmit = vi.fn();
	const props = {
		templates: [],
		isPending: false,
		error: undefined,
		onSubmit,
	};
	const { rerender } = render(<CreateNetworkPolicyForm {...props} />);
	await user.type(screen.getByRole("textbox", { name: /^Name/ }), "policy");
	rerender(<CreateNetworkPolicyForm {...props} isPending />);
	await user.type(
		screen.getByRole("textbox", { name: "Policy HCL" }),
		"ignored",
	);
	await user.click(screen.getByRole("button", { name: "Create policy" }));
	expect(onSubmit).not.toHaveBeenCalled();
	rerender(<CreateNetworkPolicyForm {...props} />);
	await user.click(screen.getByRole("button", { name: "Create policy" }));
	expect(onSubmit).toHaveBeenCalledWith(
		expect.objectContaining({ hcl: MockNetworkPolicyRevision.hcl }),
	);
});

it("keeps pending version edits read-only and discards changes back to saved HCL", async () => {
	const onSave = vi.fn();
	const user = userEvent.setup();
	const props = {
		policy: MockNetworkPolicy,
		selectedRevisionId: MockNetworkPolicyRevision.id,
		onSelectRevision: vi.fn(),
		isPending: false,
		readOnly: false,
		error: undefined,
		onSave,
	};
	const { rerender } = render(<NetworkPolicyEditor {...props} />);
	await user.clear(screen.getByRole("textbox", { name: "Policy HCL" }));
	await user.type(screen.getByRole("textbox", { name: "Policy HCL" }), "draft");
	rerender(<NetworkPolicyEditor {...props} isPending />);
	await user.type(
		screen.getByRole("textbox", { name: "Policy HCL" }),
		"ignored",
	);
	await user.click(screen.getByRole("button", { name: "Save new version" }));
	expect(onSave).not.toHaveBeenCalled();
	rerender(<NetworkPolicyEditor {...props} />);
	await user.click(screen.getByRole("button", { name: "Save new version" }));
	expect(onSave).toHaveBeenCalledWith(
		expect.objectContaining({ hcl: "draft" }),
		expect.any(Function),
	);
	await user.click(screen.getByRole("button", { name: "Discard edits" }));
	expect(screen.getByRole("region", { name: "Policy versions" })).toHaveFocus();
	await user.type(
		screen.getByRole("textbox", { name: "Policy HCL" }),
		"# comment",
	);
	await user.click(screen.getByRole("button", { name: "Save new version" }));
	expect(onSave).toHaveBeenLastCalledWith(
		expect.objectContaining({
			hcl: `${MockNetworkPolicyRevision.hcl}# comment`,
		}),
		expect.any(Function),
	);
});

it.each([false, true])(
	"allows read-only version selection after discarding any existing draft (%s)",
	async (hasDraft) => {
		const mockHistoricalRevision = {
			...MockNetworkPolicyRevision,
			id: "80000000-0000-4000-8000-000000000002",
			created_at: "2026-10-19T12:00:00Z",
			hcl: "# Historical HCL",
		};
		const props = {
			policy: {
				...MockNetworkPolicy,
				revisions: [MockNetworkPolicyRevision, mockHistoricalRevision],
			},
			selectedRevisionId: MockNetworkPolicyRevision.id,
			onSelectRevision: vi.fn(),
			isPending: false,
			readOnly: false,
			error: undefined,
			onSave: vi.fn(),
		};
		const user = userEvent.setup();
		const { rerender } = render(<NetworkPolicyEditor {...props} />);
		if (hasDraft) {
			await user.type(
				screen.getByRole("textbox", { name: "Policy HCL" }),
				"# draft",
			);
		}
		rerender(<NetworkPolicyEditor {...props} readOnly />);
		await user.type(
			screen.getByRole("textbox", { name: "Policy HCL" }),
			"ignored",
		);
		await user.click(screen.getByRole("button", { name: "Save new version" }));
		expect(props.onSave).not.toHaveBeenCalled();
		if (hasDraft) {
			await user.click(screen.getByRole("combobox", { name: "Version" }));
			expect(props.onSelectRevision).not.toHaveBeenCalled();
			await user.click(screen.getByRole("button", { name: "Discard edits" }));
		}
		await user.click(screen.getByRole("combobox", { name: "Version" }));
		await user.keyboard("{Escape}");
		await user.tab();
		expect(screen.getByRole("textbox", { name: "Policy HCL" })).toHaveFocus();
		await user.click(screen.getByRole("combobox", { name: "Version" }));
		await user.click(
			screen.getByRole("option", {
				name: `Version 1 (${mockHistoricalRevision.id.slice(0, 8)})`,
			}),
		);
		expect(props.onSelectRevision).toHaveBeenCalledWith(
			mockHistoricalRevision.id,
		);
		expect(props.onSave).not.toHaveBeenCalled();
	},
);

it("submits explicit target coordinates and provisioner tags", async () => {
	const onSubmit = vi.fn();
	const user = userEvent.setup();
	render(
		<NetworkPolicyTargetForm
			generation={7}
			isPending={false}
			error={undefined}
			onSubmit={onSubmit}
		/>,
	);
	await user.type(
		screen.getByRole("textbox", { name: /^Cluster/ }),
		"cluster-a",
	);
	await user.type(
		screen.getByRole("textbox", { name: /^Kubernetes context/ }),
		"context-a",
	);
	await user.type(
		screen.getByRole("textbox", { name: /^Namespace/ }),
		"workspaces",
	);
	await user.type(
		screen.getByRole("textbox", { name: "Provisioner tags" }),
		"region=west\npool=network",
	);
	await user.click(screen.getByRole("button", { name: "Add target" }));
	expect(onSubmit).toHaveBeenCalledWith(
		{
			name: "cluster-a/context-a/workspaces",
			cluster: "cluster-a",
			context: "context-a",
			namespace: "workspaces",
			provisioner_tags: { region: "west", pool: "network" },
			expected_generation: 7,
		},
		expect.any(Function),
	);
	await user.clear(screen.getByRole("textbox", { name: "Provisioner tags" }));
	await user.type(
		screen.getByRole("textbox", { name: "Provisioner tags" }),
		"invalid-tag",
	);
	await user.click(screen.getByRole("button", { name: "Add target" }));
	expect(onSubmit).toHaveBeenCalledTimes(1);
});

it.each([
	["Preview plan / drift", "plan"],
	["Publish selected version", "apply"],
])(
	"queues %s only after target selection and confirmation",
	async (name, operation) => {
		const onPublish = vi.fn();
		const user = userEvent.setup();
		render(
			<NetworkPolicyPublications
				policy={MockNetworkPolicy}
				selectedRevisionId={MockNetworkPolicyRevision.id}
				isPending={false}
				error={undefined}
				onPublish={onPublish}
			/>,
		);
		await user.click(screen.getByRole("button", { name }));
		expect(onPublish).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("checkbox", {
				name: `Select ${MockNetworkPolicyTarget.name}`,
			}),
		);
		await user.click(screen.getByRole("button", { name }));
		expect(onPublish).not.toHaveBeenCalled();
		await user.click(
			await screen.findByRole("button", { name: "Queue operation" }),
		);
		await waitFor(() =>
			expect(onPublish).toHaveBeenCalledWith(
				{
					operation,
					revision_id: MockNetworkPolicyRevision.id,
					target_ids: [MockNetworkPolicyTarget.id],
					expected_generation: MockNetworkPolicy.generation,
				},
				expect.any(Function),
			),
		);
	},
);

it("confirms live attachment removal and preserves its generation on refetch", async () => {
	const onSubmit = vi.fn();
	const user = userEvent.setup();
	const props = {
		policy: MockNetworkPolicy,
		templates: [MockTemplate],
		isPending: false,
		error: undefined,
		onSubmit,
	};
	const { rerender } = render(<NetworkPolicyAttachments {...props} />);
	await user.click(
		screen.getByRole("button", {
			name: `Remove ${MockTemplate.display_name || MockTemplate.name}`,
		}),
	);
	rerender(
		<NetworkPolicyAttachments
			{...props}
			policy={{ ...MockNetworkPolicy, generation: 99 }}
		/>,
	);
	await user.click(screen.getByRole("button", { name: "Save attachments" }));
	expect(onSubmit).not.toHaveBeenCalled();
	await user.click(
		await screen.findByRole("button", { name: "Update attachments" }),
	);
	expect(onSubmit).toHaveBeenCalledWith(
		{ template_ids: [], expected_generation: MockNetworkPolicy.generation },
		expect.any(Function),
	);
	await user.click(screen.getByRole("button", { name: "Cancel" }));
	await user.click(
		screen.getByRole("button", { name: "Discard attachment changes" }),
	);
	expect(
		screen.getByRole("region", { name: "Template attachments" }),
	).toHaveFocus();
});

it.each(["pending", "running", "unknown"])(
	"prevents publication when a selected target becomes %s",
	async (status) => {
		const onPublish = vi.fn();
		const user = userEvent.setup();
		const props = {
			policy: MockNetworkPolicy,
			selectedRevisionId: MockNetworkPolicyRevision.id,
			isPending: false,
			error: undefined,
			onPublish,
		};
		const { rerender } = render(<NetworkPolicyPublications {...props} />);
		await user.click(
			screen.getByRole("checkbox", {
				name: `Select ${MockNetworkPolicyTarget.name}`,
			}),
		);
		await user.click(
			screen.getByRole("button", { name: "Publish selected version" }),
		);
		rerender(
			<NetworkPolicyPublications
				{...props}
				policy={{
					...MockNetworkPolicy,
					targets: [{ ...MockNetworkPolicyTarget, status }],
				}}
			/>,
		);
		await user.click(
			await screen.findByRole("button", { name: "Queue operation" }),
		);
		expect(onPublish).not.toHaveBeenCalled();
	},
);

it("rolls back by applying a historical saved revision", async () => {
	const onPublish = vi.fn();
	const user = userEvent.setup();
	render(
		<NetworkPolicyPublications
			policy={{
				...MockNetworkPolicy,
				revisions: [
					...MockNetworkPolicy.revisions,
					{
						...MockNetworkPolicyRevision,
						id: "new-version",
						created_at: "2026-10-21T12:00:00Z",
					},
				],
			}}
			selectedRevisionId={MockNetworkPolicyRevision.id}
			isPending={false}
			error={undefined}
			onPublish={onPublish}
		/>,
	);
	await user.click(
		screen.getByRole("checkbox", {
			name: `Select ${MockNetworkPolicyTarget.name}`,
		}),
	);
	await user.click(
		screen.getByRole("button", { name: "Roll back to selected version" }),
	);
	await user.click(
		await screen.findByRole("button", { name: "Queue operation" }),
	);
	expect(onPublish).toHaveBeenCalledWith(
		expect.objectContaining({
			operation: "apply",
			revision_id: MockNetworkPolicyRevision.id,
		}),
		expect.any(Function),
	);
});
