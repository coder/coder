import { ChevronRightIcon } from "lucide-react";
import {
	type FC,
	type FormEvent,
	Fragment,
	useId,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { aiGatewayPolicy } from "#/api/queries/aiGatewayPolicies";
import type {
	AIGatewayGuardrail,
	AIGatewayGuardrailMode,
	AIGatewayHook,
	AIGatewayPipeline,
	AIGatewayPipelineGuardrail,
	AIGatewayPipelineGuardrailRequest,
	AIGatewayPipelinePolicy,
	AIGatewayPipelinePolicyRequest,
	AIGatewayPolicy,
	AIGatewayPolicyKind,
	AIProvider,
	CreateAIGatewayPipelineRequest,
	CreateAIGatewayPolicyRequest,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { cn } from "#/utils/cn";
import { RegoEditor } from "./RegoEditor";

const POLICY_KINDS: AIGatewayPolicyKind[] = [
	"classify",
	"route",
	"decide",
	"transform",
];

const HOOKS: AIGatewayHook[] = ["pre_auth", "pre_req"];

const GUARDRAIL_MODES: AIGatewayGuardrailMode[] = ["advisory", "enforcing"];

// GuardrailMemberDraft is the in-form representation of a pipeline guardrail
// member. Guardrails are always pinned at the pre-req hook in v1.
interface GuardrailMemberDraft {
	id: string;
	guardrailId: string;
	// pinnedVersionId preserves an existing member's pinned guardrail version
	// until the picker is changed. Undefined for newly added members.
	pinnedVersionId?: string;
	mode: AIGatewayGuardrailMode;
	failMode: "fail_open" | "fail_closed";
	networkTimeoutMs: number;
	enabled: boolean;
}

// Default per-guardrail network timeout. Presidio in particular lazily loads
// its NLP model on the first request, so a generous default avoids a cold-start
// timeout blocking the request when fail_closed.
const DEFAULT_GUARDRAIL_TIMEOUT_MS = 10000;

// resolveGuardrailMembers maps in-form guardrail drafts to API requests,
// pinning the selected guardrail's active version (or keeping the existing pin
// when unchanged).
function resolveGuardrailMembers(
	drafts: GuardrailMemberDraft[],
	activeGuardrails: AIGatewayGuardrail[],
	versionToGuardrailId: Map<string, string>,
): AIGatewayPipelineGuardrailRequest[] {
	const out: AIGatewayPipelineGuardrailRequest[] = [];
	for (const d of drafts) {
		const guardrail = activeGuardrails.find((g) => g.id === d.guardrailId);
		const versionId =
			d.pinnedVersionId &&
			versionToGuardrailId.get(d.pinnedVersionId) === d.guardrailId
				? d.pinnedVersionId
				: guardrail?.active_version_id;
		if (!versionId) {
			continue;
		}
		out.push({
			guardrail_version_id: versionId,
			hook: "pre_req",
			mode: d.mode,
			fail_mode: d.failMode,
			network_timeout_ms: d.networkTimeoutMs,
			enabled: d.enabled,
		});
	}
	return out;
}

// guardrailMembersToRequests maps a pipeline's existing guardrail members back
// to requests so re-minting a version (e.g. toggling a policy) preserves them.
function guardrailMembersToRequests(
	members: readonly AIGatewayPipelineGuardrail[],
): AIGatewayPipelineGuardrailRequest[] {
	return members.map((g) => ({
		guardrail_version_id: g.guardrail_version_id,
		hook: g.hook,
		mode: g.mode,
		fail_mode: g.fail_mode,
		network_timeout_ms: g.network_timeout_ms,
		enabled: g.enabled,
	}));
}

interface GuardrailMemberEditorProps {
	members: GuardrailMemberDraft[];
	guardrails: AIGatewayGuardrail[];
	onAdd: () => void;
	onUpdate: (id: string, patch: Partial<GuardrailMemberDraft>) => void;
	onRemove: (id: string) => void;
}

// GuardrailMemberEditor is the shared guardrail-attach control used by the
// create and edit pipeline dialogs. Guardrails always run at the pre-req hook.
const GuardrailMemberEditor: FC<GuardrailMemberEditorProps> = ({
	members,
	guardrails,
	onAdd,
	onUpdate,
	onRemove,
}) => (
	<div className="flex flex-col gap-2">
		<div className="flex items-center justify-between">
			<Label>Guardrails</Label>
			<Button
				type="button"
				variant="outline"
				size="sm"
				onClick={onAdd}
				disabled={guardrails.length === 0}
			>
				Add guardrail
			</Button>
		</div>
		{members.length === 0 && (
			<span className="text-xs text-content-secondary">
				No guardrails attached.
			</span>
		)}
		{members.map((member) => (
			<div
				key={member.id}
				className={cn(
					"flex items-center gap-2",
					!member.enabled && "opacity-60",
				)}
			>
				<Select
					value={member.guardrailId}
					onValueChange={(value) =>
						onUpdate(member.id, {
							guardrailId: value,
							pinnedVersionId: undefined,
						})
					}
				>
					<SelectTrigger className="flex-1">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{guardrails.map((g) => (
							<SelectItem key={g.id} value={g.id}>
								{g.display_name || g.name}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<Select
					value={member.mode}
					onValueChange={(value) =>
						onUpdate(member.id, { mode: value as AIGatewayGuardrailMode })
					}
				>
					<SelectTrigger className="w-32">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{GUARDRAIL_MODES.map((mode) => (
							<SelectItem key={mode} value={mode}>
								{mode}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<Select
					value={member.failMode}
					onValueChange={(value) =>
						onUpdate(member.id, {
							failMode: value as GuardrailMemberDraft["failMode"],
						})
					}
				>
					<SelectTrigger className="w-32">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="fail_closed">fail_closed</SelectItem>
						<SelectItem value="fail_open">fail_open</SelectItem>
					</SelectContent>
				</Select>
				<Input
					type="number"
					min={1}
					className="w-24"
					aria-label="Network timeout (ms)"
					title="Network timeout (ms)"
					value={member.networkTimeoutMs}
					onChange={(e) => {
						const ms = Number(e.target.value);
						onUpdate(member.id, {
							networkTimeoutMs: Number.isFinite(ms) && ms > 0 ? ms : 1,
						});
					}}
				/>
				<Button
					type="button"
					variant="outline"
					size="sm"
					className="w-20"
					onClick={() => onUpdate(member.id, { enabled: !member.enabled })}
				>
					{member.enabled ? "Disable" : "Enable"}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => onRemove(member.id)}
				>
					Remove
				</Button>
			</div>
		))}
	</div>
);

interface PoliciesPageViewProps {
	policies: AIGatewayPolicy[];
	pipelines: AIGatewayPipeline[];
	providers: AIProvider[];
	guardrails: AIGatewayGuardrail[];
	isLoading: boolean;
	error: unknown;
	onCreatePolicy: (
		req: CreateAIGatewayPolicyRequest,
		onSuccess: () => void,
	) => void;
	isCreating: boolean;
	createError: unknown;
	onDeletePolicy: (id: string) => void;
	deletePolicyError: unknown;
	onEditPolicy: (id: string, rego: string, onSuccess: () => void) => void;
	isEditing: boolean;
	editError: unknown;
	onRevertPolicy: (
		id: string,
		versionId: string,
		onSuccess: () => void,
	) => void;
	isReverting: boolean;
	revertError: unknown;
	onCreatePipeline: (
		req: CreateAIGatewayPipelineRequest,
		onSuccess: () => void,
	) => void;
	isCreatingPipeline: boolean;
	createPipelineError: unknown;
	onDeletePipeline: (id: string) => void;
	deletePipelineError: unknown;
	onEditPipeline: (
		id: string,
		policies: AIGatewayPipelinePolicyRequest[],
		guardrails: AIGatewayPipelineGuardrailRequest[],
		onSuccess: () => void,
	) => void;
	isEditingPipeline: boolean;
	editPipelineError: unknown;
	onTogglePipeline: (id: string, enabled: boolean) => void;
}

const PoliciesPageView: FC<PoliciesPageViewProps> = ({
	policies,
	pipelines,
	providers,
	guardrails,
	isLoading,
	error,
	onCreatePolicy,
	isCreating,
	createError,
	onDeletePolicy,
	deletePolicyError,
	onEditPolicy,
	isEditing,
	editError,
	onRevertPolicy,
	isReverting,
	revertError,
	onCreatePipeline,
	isCreatingPipeline,
	createPipelineError,
	onDeletePipeline,
	deletePipelineError,
	onEditPipeline,
	isEditingPipeline,
	editPipelineError,
	onTogglePipeline,
}) => {
	const [open, setOpen] = useState(false);
	const [pipelineOpen, setPipelineOpen] = useState(false);
	const [editingPolicyId, setEditingPolicyId] = useState<string | null>(null);
	const [editingPipeline, setEditingPipeline] =
		useState<AIGatewayPipeline | null>(null);
	const [expandedPipelines, setExpandedPipelines] = useState<Set<string>>(
		new Set(),
	);

	// Resolve a pinned policy_version_id back to its parent policy and version
	// number so pipeline members can be shown by name and opened for revert.
	const versionToPolicy = new Map<
		string,
		{ policy: AIGatewayPolicy; versionNumber: number }
	>();
	for (const p of policies) {
		for (const v of p.versions ?? []) {
			versionToPolicy.set(v.id, { policy: p, versionNumber: v.version_number });
		}
	}

	// Only guardrails with an active version can be attached.
	const activeGuardrails = guardrails.filter((g) => g.active_version_id);
	const versionToGuardrailId = new Map<string, string>();
	const versionToGuardrail = new Map<
		string,
		{ guardrail: AIGatewayGuardrail; versionNumber: number }
	>();
	for (const g of guardrails) {
		for (const v of g.versions ?? []) {
			versionToGuardrailId.set(v.id, g.id);
			versionToGuardrail.set(v.id, {
				guardrail: g,
				versionNumber: v.version_number,
			});
		}
	}

	const togglePipeline = (id: string) =>
		setExpandedPipelines((prev) => {
			const next = new Set(prev);
			if (next.has(id)) {
				next.delete(id);
			} else {
				next.add(id);
			}
			return next;
		});

	// Enable/disable a policy within a pipeline by minting a new pipeline version
	// that flips that tuple's enabled flag in ai_gateway_pipeline_version_policies.
	// It never touches the policy itself.
	const toggleAttachedPolicy = (
		pipeline: AIGatewayPipeline,
		target: AIGatewayPipelinePolicy,
	) => {
		const next = (pipeline.active_version?.policies ?? []).map((m) => ({
			policy_version_id: m.policy_version_id,
			hook: m.hook,
			fail_mode: m.fail_mode,
			enabled:
				m.policy_version_id === target.policy_version_id &&
				m.hook === target.hook
					? !m.enabled
					: m.enabled,
		}));
		// Preserve the pipeline's guardrail members; re-minting a version with
		// only policies would drop them.
		onEditPipeline(
			pipeline.id,
			next,
			guardrailMembersToRequests(pipeline.active_version?.guardrails ?? []),
			() => {},
		);
	};

	// Enable/disable a guardrail within a pipeline by re-minting the version
	// with that member's enabled flag flipped, preserving all other members.
	const toggleAttachedGuardrail = (
		pipeline: AIGatewayPipeline,
		target: AIGatewayPipelineGuardrail,
	) => {
		const nextGuardrails = (pipeline.active_version?.guardrails ?? []).map(
			(m) => ({
				guardrail_version_id: m.guardrail_version_id,
				hook: m.hook,
				mode: m.mode,
				fail_mode: m.fail_mode,
				network_timeout_ms: m.network_timeout_ms,
				enabled:
					m.guardrail_version_id === target.guardrail_version_id &&
					m.hook === target.hook
						? !m.enabled
						: m.enabled,
			}),
		);
		const keepPolicies = (pipeline.active_version?.policies ?? []).map((m) => ({
			policy_version_id: m.policy_version_id,
			hook: m.hook,
			fail_mode: m.fail_mode,
			enabled: m.enabled,
		}));
		onEditPipeline(pipeline.id, keepPolicies, nextGuardrails, () => {});
	};

	return (
		<div className="flex flex-col gap-8">
			<hr className="order-2 m-0 w-full border-0 border-t border-solid border-border" />
			<div className="order-3">
				<SettingsHeader
					actions={
						<CreatePolicyDialog
							open={open}
							onOpenChange={setOpen}
							onCreatePolicy={onCreatePolicy}
							isCreating={isCreating}
							createError={createError}
						/>
					}
				>
					<SettingsHeaderTitle>Policies</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Reusable, versioned Rego policies evaluated inline by the AI
						gateway. Attach policies to a provider via a pipeline.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{Boolean(error) && (
					<div className="mb-4">
						<ErrorAlert error={error} />
					</div>
				)}
				{Boolean(deletePolicyError) && (
					<div className="mb-4">
						<ErrorAlert error={deletePolicyError} />
					</div>
				)}

				<Table aria-label="AI gateway policies">
					<TableHeader>
						<TableRow>
							<TableHead className="w-1/3">Name</TableHead>
							<TableHead>Kind</TableHead>
							<TableHead>Active version</TableHead>
							<TableHead className="w-44" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableLoader />
						) : policies.length === 0 ? (
							<TableEmpty message="No policies configured" isCompact />
						) : (
							policies.map((policy) => (
								<TableRow key={policy.id}>
									<TableCell>{policy.display_name || policy.name}</TableCell>
									<TableCell>
										<Badge size="sm">{policy.kind}</Badge>
									</TableCell>
									<TableCell>{activeVersionLabel(policy)}</TableCell>
									<TableCell>
										<div className="flex justify-end gap-2">
											<Button
												variant="outline"
												size="sm"
												onClick={() => setEditingPolicyId(policy.id)}
											>
												Edit
											</Button>
											<Button
												variant="outline"
												size="sm"
												onClick={() => onDeletePolicy(policy.id)}
											>
												Delete
											</Button>
										</div>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>

			<div className="order-1">
				<SettingsHeader
					actions={
						<CreatePipelineDialog
							open={pipelineOpen}
							onOpenChange={setPipelineOpen}
							providers={providers}
							policies={policies}
							pipelines={pipelines}
							guardrails={activeGuardrails}
							versionToGuardrailId={versionToGuardrailId}
							onCreatePipeline={onCreatePipeline}
							isCreating={isCreatingPipeline}
							createError={createPipelineError}
						/>
					}
				>
					<SettingsHeaderTitle>Pipelines</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Each provider has at most one pipeline. A pipeline pins policy
						versions to hooks and is swapped atomically.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{Boolean(deletePipelineError) && (
					<div className="mb-4">
						<ErrorAlert error={deletePipelineError} />
					</div>
				)}

				<Table aria-label="AI gateway pipelines">
					<TableHeader>
						<TableRow>
							<TableHead className="w-1/2">Provider</TableHead>
							<TableHead>Status</TableHead>
							<TableHead>Policies</TableHead>
							<TableHead className="w-72" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableLoader />
						) : pipelines.length === 0 ? (
							<TableEmpty message="No pipelines configured" isCompact />
						) : (
							pipelines.map((pipeline) => {
								const members = pipeline.active_version?.policies ?? [];
								const pipelineGuardrails =
									pipeline.active_version?.guardrails ?? [];
								const isOpen = expandedPipelines.has(pipeline.id);
								return (
									<Fragment key={pipeline.id}>
										<TableRow>
											<TableCell>
												<Button
													variant="subtle"
													size="sm"
													className="h-auto min-w-0 p-0 align-middle font-medium text-content-primary"
													onClick={() => togglePipeline(pipeline.id)}
												>
													<ChevronRightIcon
														className={cn(
															"mr-1 transition-transform",
															isOpen && "rotate-90",
														)}
													/>
													<span className="sr-only">
														({isOpen ? "Hide" : "Show"} policies)
													</span>
													{providerName(providers, pipeline.provider_id)}
												</Button>
											</TableCell>
											<TableCell>
												<Badge
													size="sm"
													variant={pipeline.enabled ? "green" : "default"}
												>
													{pipeline.enabled ? "Enabled" : "Disabled"}
												</Badge>
											</TableCell>
											<TableCell>{members.length}</TableCell>
											<TableCell>
												<div className="flex justify-end gap-2">
													<Button
														variant="outline"
														size="sm"
														onClick={() => setEditingPipeline(pipeline)}
													>
														Edit policies
													</Button>
													<Button
														variant="outline"
														size="sm"
														onClick={() =>
															onTogglePipeline(pipeline.id, !pipeline.enabled)
														}
													>
														{pipeline.enabled ? "Disable" : "Enable"}
													</Button>
													<Button
														variant="outline"
														size="sm"
														onClick={() => onDeletePipeline(pipeline.id)}
													>
														Delete
													</Button>
												</div>
											</TableCell>
										</TableRow>
										{isOpen && (
											<TableRow>
												<TableCell colSpan={4} className="bg-surface-secondary">
													{members.length === 0 ? (
														<span className="text-xs text-content-secondary">
															No policies attached.
														</span>
													) : (
														<div className="flex flex-col gap-1">
															{members.map((member) => {
																const resolved = versionToPolicy.get(
																	member.policy_version_id,
																);
																return (
																	<div
																		key={member.policy_version_id}
																		className="flex items-center justify-between gap-2"
																	>
																		<span className="text-sm">
																			<span className="font-medium">
																				{resolved?.policy.name ??
																					"unknown policy"}
																			</span>{" "}
																			<span className="text-content-secondary">
																				{member.kind} · {member.hook} ·{" "}
																				{member.fail_mode}
																				{resolved
																					? ` · v${resolved.versionNumber}`
																					: ""}
																			</span>{" "}
																			{!member.enabled && (
																				<Badge size="sm" variant="default">
																					Disabled
																				</Badge>
																			)}
																		</span>
																		<div className="flex items-center gap-2">
																			<Button
																				variant="outline"
																				size="sm"
																				onClick={() =>
																					toggleAttachedPolicy(pipeline, member)
																				}
																			>
																				{member.enabled ? "Disable" : "Enable"}
																			</Button>
																			{resolved && (
																				<Button
																					variant="outline"
																					size="sm"
																					onClick={() =>
																						setEditingPolicyId(
																							resolved.policy.id,
																						)
																					}
																				>
																					Edit / revert
																				</Button>
																			)}
																		</div>
																	</div>
																);
															})}
														</div>
													)}
													{pipelineGuardrails.length > 0 && (
														<div className="mt-3 flex flex-col gap-1 border-0 border-t border-solid border-border pt-3">
															<span className="text-xs font-medium text-content-secondary">
																Guardrails
															</span>
															{pipelineGuardrails.map((g) => {
																const resolved = versionToGuardrail.get(
																	g.guardrail_version_id,
																);
																return (
																	<div
																		key={g.guardrail_version_id}
																		className="flex items-center justify-between gap-2"
																	>
																		<span className="text-sm">
																			<span className="font-medium">
																				{resolved?.guardrail.name ??
																					"unknown guardrail"}
																			</span>{" "}
																			<span className="text-content-secondary">
																				{g.mode} · {g.hook} · {g.fail_mode}
																				{resolved
																					? ` · v${resolved.versionNumber}`
																					: ""}
																			</span>{" "}
																			{!g.enabled && (
																				<Badge size="sm" variant="default">
																					Disabled
																				</Badge>
																			)}
																		</span>
																		<Button
																			variant="outline"
																			size="sm"
																			onClick={() =>
																				toggleAttachedGuardrail(pipeline, g)
																			}
																		>
																			{g.enabled ? "Disable" : "Enable"}
																		</Button>
																	</div>
																);
															})}
														</div>
													)}
												</TableCell>
											</TableRow>
										)}
									</Fragment>
								);
							})
						)}
					</TableBody>
				</Table>
			</div>

			<EditPolicyDialog
				policyId={editingPolicyId}
				onClose={() => setEditingPolicyId(null)}
				onSave={onEditPolicy}
				isSaving={isEditing}
				error={editError}
				onRevert={onRevertPolicy}
				isReverting={isReverting}
				revertError={revertError}
			/>

			<EditPipelineDialog
				pipeline={editingPipeline}
				policies={policies}
				guardrails={activeGuardrails}
				versionToGuardrailId={versionToGuardrailId}
				onClose={() => setEditingPipeline(null)}
				onSave={onEditPipeline}
				isSaving={isEditingPipeline}
				error={editPipelineError}
			/>
		</div>
	);
};

interface CreatePolicyDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onCreatePolicy: (
		req: CreateAIGatewayPolicyRequest,
		onSuccess: () => void,
	) => void;
	isCreating: boolean;
	createError: unknown;
}

const CreatePolicyDialog: FC<CreatePolicyDialogProps> = ({
	open,
	onOpenChange,
	onCreatePolicy,
	isCreating,
	createError,
}) => {
	const nameId = useId();
	const kindId = useId();
	const [name, setName] = useState("");
	const [kind, setKind] = useState<AIGatewayPolicyKind>("decide");
	const [rego, setRego] = useState('default verdict := "ALLOW"');

	const submit = (e: FormEvent) => {
		e.preventDefault();
		onCreatePolicy({ name, kind, rego }, () => {
			onOpenChange(false);
			setName("");
			setRego('default verdict := "ALLOW"');
		});
	};

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogTrigger asChild>
				<Button variant="outline">Create policy</Button>
			</DialogTrigger>
			<DialogContent className="max-w-4xl">
				<form onSubmit={submit} className="flex flex-col gap-4">
					<DialogHeader>
						<DialogTitle>Create policy</DialogTitle>
						<DialogDescription>
							The Rego is validated on save against the selected kind.
						</DialogDescription>
					</DialogHeader>

					{Boolean(createError) && <ErrorAlert error={createError} />}

					<div className="flex flex-col gap-2">
						<Label htmlFor={nameId}>Name</Label>
						<Input
							id={nameId}
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder="model-allowlist"
							required
						/>
					</div>

					<div className="flex flex-col gap-2">
						<Label htmlFor={kindId}>Kind</Label>
						<Select
							value={kind}
							onValueChange={(value) => setKind(value as AIGatewayPolicyKind)}
						>
							<SelectTrigger id={kindId}>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{POLICY_KINDS.map((k) => (
									<SelectItem key={k} value={k}>
										{k}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>

					<div className="flex flex-col gap-2">
						<Label>Rego</Label>
						<RegoEditor value={rego} onChange={setRego} ariaLabel="Rego" />
					</div>

					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={isCreating}>
							<Spinner loading={isCreating} />
							Create
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};

function providerName(providers: AIProvider[], id: string): string {
	const match = providers.find((p) => p.id === id);
	return match ? match.display_name || match.name : id;
}

interface PipelineMemberDraft {
	id: string;
	policyId: string;
	// pinnedVersionId preserves an existing member's pinned policy version until
	// the policy picker is changed. Undefined for newly added members.
	pinnedVersionId?: string;
	hook: AIGatewayHook;
	failMode: "fail_open" | "fail_closed";
	enabled: boolean;
}

interface CreatePipelineDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	providers: AIProvider[];
	policies: AIGatewayPolicy[];
	pipelines: AIGatewayPipeline[];
	guardrails: AIGatewayGuardrail[];
	versionToGuardrailId: Map<string, string>;
	onCreatePipeline: (
		req: CreateAIGatewayPipelineRequest,
		onSuccess: () => void,
	) => void;
	isCreating: boolean;
	createError: unknown;
}

const CreatePipelineDialog: FC<CreatePipelineDialogProps> = ({
	open,
	onOpenChange,
	providers,
	policies,
	pipelines,
	guardrails,
	versionToGuardrailId,
	onCreatePipeline,
	isCreating,
	createError,
}) => {
	const providerId = useId();
	// Providers that do not already have a pipeline.
	const usedProviders = new Set(pipelines.map((p) => p.provider_id));
	const availableProviders = providers.filter((p) => !usedProviders.has(p.id));
	// Only policies with an active version can be pinned.
	const activePolicies = policies.filter((p) => p.active_version_id);

	const [provider, setProvider] = useState("");
	const [members, setMembers] = useState<PipelineMemberDraft[]>([]);
	const [guardrailMembers, setGuardrailMembers] = useState<
		GuardrailMemberDraft[]
	>([]);
	const nextMemberId = useRef(0);

	const addMember = () => {
		const first = activePolicies[0];
		if (!first) {
			return;
		}
		nextMemberId.current += 1;
		const id = String(nextMemberId.current);
		setMembers((prev) => [
			...prev,
			{
				id,
				policyId: first.id,
				hook: "pre_req",
				failMode: "fail_closed",
				enabled: true,
			},
		]);
	};

	const updateMember = (id: string, patch: Partial<PipelineMemberDraft>) => {
		setMembers((prev) =>
			prev.map((m) => (m.id === id ? { ...m, ...patch } : m)),
		);
	};

	const removeMember = (id: string) => {
		setMembers((prev) => prev.filter((m) => m.id !== id));
	};

	const addGuardrail = () => {
		const first = guardrails[0];
		if (!first) {
			return;
		}
		nextMemberId.current += 1;
		setGuardrailMembers((prev) => [
			...prev,
			{
				id: `g${nextMemberId.current}`,
				guardrailId: first.id,
				mode: "enforcing",
				failMode: "fail_closed",
				networkTimeoutMs: DEFAULT_GUARDRAIL_TIMEOUT_MS,
				enabled: true,
			},
		]);
	};

	const updateGuardrail = (id: string, patch: Partial<GuardrailMemberDraft>) =>
		setGuardrailMembers((prev) =>
			prev.map((m) => (m.id === id ? { ...m, ...patch } : m)),
		);

	const removeGuardrail = (id: string) =>
		setGuardrailMembers((prev) => prev.filter((m) => m.id !== id));

	const submit = (e: FormEvent) => {
		e.preventDefault();
		const resolved: AIGatewayPipelinePolicyRequest[] = [];
		for (const m of members) {
			const policy = activePolicies.find((p) => p.id === m.policyId);
			if (!policy?.active_version_id) {
				continue;
			}
			resolved.push({
				policy_version_id: policy.active_version_id,
				hook: m.hook,
				fail_mode: m.failMode,
				enabled: m.enabled,
			});
		}
		onCreatePipeline(
			{
				provider_id: provider,
				enabled: true,
				policies: resolved,
				guardrails: resolveGuardrailMembers(
					guardrailMembers,
					guardrails,
					versionToGuardrailId,
				),
			},
			() => {
				onOpenChange(false);
				setProvider("");
				setMembers([]);
				setGuardrailMembers([]);
			},
		);
	};

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogTrigger asChild>
				<Button variant="outline" disabled={availableProviders.length === 0}>
					Create pipeline
				</Button>
			</DialogTrigger>
			<DialogContent className="max-w-3xl">
				<form onSubmit={submit} className="flex flex-col gap-4">
					<DialogHeader>
						<DialogTitle>Create pipeline</DialogTitle>
						<DialogDescription>
							Attach policies to a provider. Each policy is pinned at its active
							version.
						</DialogDescription>
					</DialogHeader>

					{Boolean(createError) && <ErrorAlert error={createError} />}

					<div className="flex flex-col gap-2">
						<Label htmlFor={providerId}>Provider</Label>
						<Select value={provider} onValueChange={setProvider}>
							<SelectTrigger id={providerId}>
								<SelectValue placeholder="Select a provider" />
							</SelectTrigger>
							<SelectContent>
								{availableProviders.map((p) => (
									<SelectItem key={p.id} value={p.id}>
										{p.display_name || p.name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>

					<div className="flex flex-col gap-2">
						<div className="flex items-center justify-between">
							<Label>Policies</Label>
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={addMember}
								disabled={activePolicies.length === 0}
							>
								Add policy
							</Button>
						</div>
						{members.length === 0 && (
							<span className="text-xs text-content-secondary">
								No policies added yet.
							</span>
						)}
						{members.map((member) => (
							<div key={member.id} className="flex items-center gap-2">
								<Select
									value={member.policyId}
									onValueChange={(value) =>
										updateMember(member.id, { policyId: value })
									}
								>
									<SelectTrigger className="flex-1">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{activePolicies.map((p) => (
											<SelectItem key={p.id} value={p.id}>
												{p.display_name || p.name} ({p.kind})
											</SelectItem>
										))}
									</SelectContent>
								</Select>
								<Select
									value={member.hook}
									onValueChange={(value) =>
										updateMember(member.id, { hook: value as AIGatewayHook })
									}
								>
									<SelectTrigger className="w-28">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{HOOKS.map((h) => (
											<SelectItem key={h} value={h}>
												{h}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
								<Select
									value={member.failMode}
									onValueChange={(value) =>
										updateMember(member.id, {
											failMode: value as PipelineMemberDraft["failMode"],
										})
									}
								>
									<SelectTrigger className="w-32">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="fail_closed">fail_closed</SelectItem>
										<SelectItem value="fail_open">fail_open</SelectItem>
									</SelectContent>
								</Select>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={() => removeMember(member.id)}
								>
									Remove
								</Button>
							</div>
						))}
					</div>

					<GuardrailMemberEditor
						members={guardrailMembers}
						guardrails={guardrails}
						onAdd={addGuardrail}
						onUpdate={updateGuardrail}
						onRemove={removeGuardrail}
					/>

					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={isCreating || provider === ""}>
							<Spinner loading={isCreating} />
							Create
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};

function activeRego(policy: AIGatewayPolicy): string {
	const versions = policy.versions ?? [];
	const active = versions.find((v) => v.id === policy.active_version_id);
	return active?.rego ?? versions[0]?.rego ?? "";
}

function activeVersionLabel(policy: AIGatewayPolicy): string {
	const active = (policy.versions ?? []).find(
		(v) => v.id === policy.active_version_id,
	);
	return active ? `v${active.version_number}` : "None";
}

interface EditPolicyDialogProps {
	policyId: string | null;
	onClose: () => void;
	onSave: (id: string, rego: string, onSuccess: () => void) => void;
	isSaving: boolean;
	error: unknown;
	onRevert: (id: string, versionId: string, onSuccess: () => void) => void;
	isReverting: boolean;
	revertError: unknown;
}

const EditPolicyDialog: FC<EditPolicyDialogProps> = ({
	policyId,
	onClose,
	onSave,
	isSaving,
	error,
	onRevert,
	isReverting,
	revertError,
}) => {
	// Load the full policy (with versions/rego); the list response may omit them.
	const detailQuery = useQuery({
		...aiGatewayPolicy(policyId ?? ""),
		enabled: policyId !== null,
	});

	return (
		<Dialog
			open={policyId !== null}
			onOpenChange={(next) => {
				if (!next) {
					onClose();
				}
			}}
		>
			<DialogContent className="max-w-4xl">
				{detailQuery.data ? (
					<EditPolicyForm
						key={detailQuery.data.active_version_id ?? detailQuery.data.id}
						policy={detailQuery.data}
						onClose={onClose}
						onSave={onSave}
						isSaving={isSaving}
						error={error}
						onRevert={onRevert}
						isReverting={isReverting}
						revertError={revertError}
					/>
				) : (
					<div className="flex h-64 items-center justify-center">
						<Spinner loading />
					</div>
				)}
			</DialogContent>
		</Dialog>
	);
};

interface EditPolicyFormProps {
	policy: AIGatewayPolicy;
	onClose: () => void;
	onSave: (id: string, rego: string, onSuccess: () => void) => void;
	isSaving: boolean;
	error: unknown;
	onRevert: (id: string, versionId: string, onSuccess: () => void) => void;
	isReverting: boolean;
	revertError: unknown;
}

const EditPolicyForm: FC<EditPolicyFormProps> = ({
	policy,
	onClose,
	onSave,
	isSaving,
	error,
	onRevert,
	isReverting,
	revertError,
}) => {
	const [rego, setRego] = useState(() => activeRego(policy));
	const [expanded, setExpanded] = useState<Set<string>>(new Set());

	const submit = (e: FormEvent) => {
		e.preventDefault();
		onSave(policy.id, rego, onClose);
	};

	// Most recent versions first; the list response orders by version desc.
	const recentVersions = (policy.versions ?? []).slice(0, 3);

	const toggleExpanded = (id: string) => {
		setExpanded((prev) => {
			const next = new Set(prev);
			if (next.has(id)) {
				next.delete(id);
			} else {
				next.add(id);
			}
			return next;
		});
	};

	return (
		<form onSubmit={submit} className="flex flex-col gap-4">
			<DialogHeader>
				<DialogTitle>Edit {policy.display_name || policy.name}</DialogTitle>
				<DialogDescription>
					Saving creates a new active version. Earlier versions are retained.
				</DialogDescription>
			</DialogHeader>

			{Boolean(error) && <ErrorAlert error={error} />}

			<div className="flex flex-col gap-2">
				<Label>Rego</Label>
				<RegoEditor
					value={rego}
					onChange={setRego}
					ariaLabel="Rego"
					height={480}
				/>
			</div>

			<DialogFooter>
				<Button type="button" variant="outline" onClick={onClose}>
					Cancel
				</Button>
				<Button type="submit" disabled={isSaving}>
					<Spinner loading={isSaving} />
					Save new version
				</Button>
			</DialogFooter>

			<div className="flex flex-col gap-2 border-t border-border pt-4">
				<span className="text-sm font-medium">Version history</span>
				{Boolean(revertError) && <ErrorAlert error={revertError} />}
				{recentVersions.map((version) => {
					const isActive = version.id === policy.active_version_id;
					const isOpen = expanded.has(version.id);
					return (
						<div
							key={version.id}
							className="rounded border border-solid border-border"
						>
							<div className="flex items-center justify-between gap-2 px-3 py-2">
								<Button
									variant="subtle"
									size="sm"
									className="h-auto min-w-0 gap-2 p-0 text-content-primary"
									onClick={() => toggleExpanded(version.id)}
								>
									<ChevronRightIcon
										className={cn(
											"transition-transform",
											isOpen && "rotate-90",
										)}
									/>
									<span className="font-medium">v{version.version_number}</span>
									{isActive && (
										<Badge size="sm" variant="green">
											Active
										</Badge>
									)}
									<span className="text-xs text-content-secondary">
										{new Date(version.created_at).toLocaleString("en-US")}
									</span>
								</Button>
								{!isActive && (
									<Button
										type="button"
										variant="outline"
										size="sm"
										disabled={isReverting}
										onClick={() => onRevert(policy.id, version.id, onClose)}
									>
										Revert to this
									</Button>
								)}
							</div>
							{isOpen && (
								<pre className="m-0 overflow-auto border-t border-border bg-surface-secondary p-3 font-mono text-xs">
									{version.rego}
								</pre>
							)}
						</div>
					);
				})}
			</div>
		</form>
	);
};

interface EditPipelineDialogProps {
	pipeline: AIGatewayPipeline | null;
	policies: AIGatewayPolicy[];
	guardrails: AIGatewayGuardrail[];
	versionToGuardrailId: Map<string, string>;
	onClose: () => void;
	onSave: (
		id: string,
		policies: AIGatewayPipelinePolicyRequest[],
		guardrails: AIGatewayPipelineGuardrailRequest[],
		onSuccess: () => void,
	) => void;
	isSaving: boolean;
	error: unknown;
}

const EditPipelineDialog: FC<EditPipelineDialogProps> = ({
	pipeline,
	policies,
	guardrails,
	versionToGuardrailId,
	onClose,
	onSave,
	isSaving,
	error,
}) => {
	return (
		<Dialog
			open={pipeline !== null}
			onOpenChange={(next) => {
				if (!next) {
					onClose();
				}
			}}
		>
			<DialogContent className="max-w-3xl">
				{pipeline && (
					<EditPipelineForm
						key={pipeline.active_version_id ?? pipeline.id}
						pipeline={pipeline}
						policies={policies}
						guardrails={guardrails}
						versionToGuardrailId={versionToGuardrailId}
						onClose={onClose}
						onSave={onSave}
						isSaving={isSaving}
						error={error}
					/>
				)}
			</DialogContent>
		</Dialog>
	);
};

interface EditPipelineFormProps {
	pipeline: AIGatewayPipeline;
	policies: AIGatewayPolicy[];
	guardrails: AIGatewayGuardrail[];
	versionToGuardrailId: Map<string, string>;
	onClose: () => void;
	onSave: (
		id: string,
		policies: AIGatewayPipelinePolicyRequest[],
		guardrails: AIGatewayPipelineGuardrailRequest[],
		onSuccess: () => void,
	) => void;
	isSaving: boolean;
	error: unknown;
}

const EditPipelineForm: FC<EditPipelineFormProps> = ({
	pipeline,
	policies,
	guardrails,
	versionToGuardrailId,
	onClose,
	onSave,
	isSaving,
	error,
}) => {
	const activePolicies = policies.filter((p) => p.active_version_id);
	const versionToPolicyId = new Map<string, string>();
	for (const p of policies) {
		for (const v of p.versions ?? []) {
			versionToPolicyId.set(v.id, p.id);
		}
	}

	const nextId = useRef(0);
	const makeId = () => {
		nextId.current += 1;
		return String(nextId.current);
	};

	const [members, setMembers] = useState<PipelineMemberDraft[]>(() =>
		(pipeline.active_version?.policies ?? []).map((m) => ({
			id: makeId(),
			// Show the parent policy in the picker; existing entries keep their
			// pinned version until the picker is changed.
			policyId: versionToPolicyId.get(m.policy_version_id) ?? "",
			pinnedVersionId: m.policy_version_id,
			hook: m.hook,
			failMode: m.fail_mode === "fail_open" ? "fail_open" : "fail_closed",
			enabled: m.enabled,
		})),
	);

	const [guardrailMembers, setGuardrailMembers] = useState<
		GuardrailMemberDraft[]
	>(() =>
		(pipeline.active_version?.guardrails ?? []).map((m) => ({
			id: `g${makeId()}`,
			guardrailId: versionToGuardrailId.get(m.guardrail_version_id) ?? "",
			pinnedVersionId: m.guardrail_version_id,
			mode: m.mode,
			failMode: m.fail_mode === "fail_open" ? "fail_open" : "fail_closed",
			networkTimeoutMs: m.network_timeout_ms,
			enabled: m.enabled,
		})),
	);

	const addGuardrail = () => {
		const first = guardrails[0];
		if (!first) {
			return;
		}
		setGuardrailMembers((prev) => [
			...prev,
			{
				id: `g${makeId()}`,
				guardrailId: first.id,
				pinnedVersionId: undefined,
				mode: "enforcing",
				failMode: "fail_closed",
				networkTimeoutMs: DEFAULT_GUARDRAIL_TIMEOUT_MS,
				enabled: true,
			},
		]);
	};

	const updateGuardrail = (id: string, patch: Partial<GuardrailMemberDraft>) =>
		setGuardrailMembers((prev) =>
			prev.map((m) => (m.id === id ? { ...m, ...patch } : m)),
		);

	const removeGuardrail = (id: string) =>
		setGuardrailMembers((prev) => prev.filter((m) => m.id !== id));

	const addMember = () => {
		const first = activePolicies[0];
		if (!first) {
			return;
		}
		setMembers((prev) => [
			...prev,
			{
				id: makeId(),
				policyId: first.id,
				pinnedVersionId: undefined,
				hook: "pre_req",
				failMode: "fail_closed",
				enabled: true,
			},
		]);
	};

	const updateMember = (id: string, patch: Partial<PipelineMemberDraft>) =>
		setMembers((prev) =>
			prev.map((m) => (m.id === id ? { ...m, ...patch } : m)),
		);

	const removeMember = (id: string) =>
		setMembers((prev) => prev.filter((m) => m.id !== id));

	const submit = (e: FormEvent) => {
		e.preventDefault();
		const resolved: AIGatewayPipelinePolicyRequest[] = [];
		for (const m of members) {
			const policy = activePolicies.find((p) => p.id === m.policyId);
			// Keep the existing pinned version when the picker was not changed,
			// otherwise pin the selected policy's active version.
			const versionId =
				m.pinnedVersionId &&
				versionToPolicyId.get(m.pinnedVersionId) === m.policyId
					? m.pinnedVersionId
					: policy?.active_version_id;
			if (!versionId) {
				continue;
			}
			resolved.push({
				policy_version_id: versionId,
				hook: m.hook,
				fail_mode: m.failMode,
				enabled: m.enabled,
			});
		}
		onSave(
			pipeline.id,
			resolved,
			resolveGuardrailMembers(
				guardrailMembers,
				guardrails,
				versionToGuardrailId,
			),
			onClose,
		);
	};

	return (
		<form onSubmit={submit} className="flex flex-col gap-4">
			<DialogHeader>
				<DialogTitle>Edit pipeline policies</DialogTitle>
				<DialogDescription>
					Saving creates a new active pipeline version with this policy set.
				</DialogDescription>
			</DialogHeader>

			{Boolean(error) && <ErrorAlert error={error} />}

			<div className="flex flex-col gap-2">
				<div className="flex items-center justify-between">
					<Label>Policies</Label>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={addMember}
						disabled={activePolicies.length === 0}
					>
						Add policy
					</Button>
				</div>
				{members.length === 0 && (
					<span className="text-xs text-content-secondary">
						No policies attached.
					</span>
				)}
				{members.map((member) => (
					<div
						key={member.id}
						className={cn(
							"flex items-center gap-2",
							!member.enabled && "opacity-60",
						)}
					>
						<Select
							value={member.policyId}
							onValueChange={(value) =>
								updateMember(member.id, {
									policyId: value,
									pinnedVersionId: undefined,
								})
							}
						>
							<SelectTrigger className="flex-1">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{activePolicies.map((p) => (
									<SelectItem key={p.id} value={p.id}>
										{p.display_name || p.name} ({p.kind})
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						<Select
							value={member.hook}
							onValueChange={(value) =>
								updateMember(member.id, { hook: value as AIGatewayHook })
							}
						>
							<SelectTrigger className="w-28">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{HOOKS.map((h) => (
									<SelectItem key={h} value={h}>
										{h}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						<Select
							value={member.failMode}
							onValueChange={(value) =>
								updateMember(member.id, {
									failMode: value as PipelineMemberDraft["failMode"],
								})
							}
						>
							<SelectTrigger className="w-32">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="fail_closed">fail_closed</SelectItem>
								<SelectItem value="fail_open">fail_open</SelectItem>
							</SelectContent>
						</Select>
						<Button
							type="button"
							variant="outline"
							size="sm"
							className="w-20"
							onClick={() =>
								updateMember(member.id, { enabled: !member.enabled })
							}
						>
							{member.enabled ? "Disable" : "Enable"}
						</Button>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={() => removeMember(member.id)}
						>
							Remove
						</Button>
					</div>
				))}
			</div>

			<GuardrailMemberEditor
				members={guardrailMembers}
				guardrails={guardrails}
				onAdd={addGuardrail}
				onUpdate={updateGuardrail}
				onRemove={removeGuardrail}
			/>

			<DialogFooter>
				<Button type="button" variant="outline" onClick={onClose}>
					Cancel
				</Button>
				<Button type="submit" disabled={isSaving}>
					<Spinner loading={isSaving} />
					Save new version
				</Button>
			</DialogFooter>
		</form>
	);
};

export default PoliciesPageView;
