import { type FC, type FormEvent, useId, useState } from "react";
import type {
	AIGatewayGuardrail,
	CreateAIGatewayGuardrailRequest,
	CreateAIGatewayGuardrailVersionRequest,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
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

const PRESIDIO_CONFIG_EXAMPLE = JSON.stringify(
	{
		analyzer_url: "http://localhost:5002",
		anonymizer_url: "http://localhost:5001",
		entity_actions: {
			EMAIL_ADDRESS: "MASK",
			US_SSN: "BLOCK",
		},
	},
	null,
	2,
);

interface GuardrailsSectionProps {
	guardrails: readonly AIGatewayGuardrail[];
	isLoading: boolean;
	error: unknown;
	onCreate: (
		request: CreateAIGatewayGuardrailRequest,
		onSuccess: () => void,
	) => void;
	isCreating: boolean;
	createError: unknown;
	onEdit: (
		id: string,
		request: CreateAIGatewayGuardrailVersionRequest,
		onSuccess: () => void,
	) => void;
	isEditing: boolean;
	editError: unknown;
	onDelete: (id: string) => void;
	deleteError: unknown;
	onToggle: (id: string, enabled: boolean) => void;
}

/**
 * GuardrailsSection renders networked guardrail management as a subsection of
 * the AI gateway policies page. Guardrails are configured here and attached to a
 * provider via a pipeline.
 */
export const GuardrailsSection: FC<GuardrailsSectionProps> = ({
	guardrails,
	isLoading,
	error,
	onCreate,
	isCreating,
	createError,
	onEdit,
	isEditing,
	editError,
	onDelete,
	deleteError,
	onToggle,
}) => {
	const [createOpen, setCreateOpen] = useState(false);
	const [editing, setEditing] = useState<AIGatewayGuardrail | null>(null);

	return (
		<section className="flex flex-col gap-4">
			<SettingsHeader
				actions={
					<Dialog open={createOpen} onOpenChange={setCreateOpen}>
						<DialogTrigger asChild>
							<Button variant="outline">Create guardrail</Button>
						</DialogTrigger>
						<CreateGuardrailDialog
							isCreating={isCreating}
							createError={createError}
							onCreate={(req) => onCreate(req, () => setCreateOpen(false))}
						/>
					</Dialog>
				}
			>
				<SettingsHeaderTitle>Guardrails</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Networked safety and DLP checks (e.g. Presidio) that run inline on
					requests. Attach a guardrail to a provider pipeline to enforce it.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{error ? <ErrorAlert error={error} /> : null}
			{deleteError ? <ErrorAlert error={deleteError} /> : null}

			<EditGuardrailDialog
				guardrail={editing}
				isEditing={isEditing}
				editError={editError}
				onClose={() => setEditing(null)}
				onEdit={onEdit}
			/>

			<Table aria-label="Guardrails">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Adapter</TableHead>
						<TableHead>Active version</TableHead>
						<TableHead>Status</TableHead>
						<TableHead className="w-0" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading ? (
						<TableLoader />
					) : guardrails.length === 0 ? (
						<TableEmpty message="No guardrails configured" isCompact />
					) : (
						guardrails.map((g) => {
							const activeVersion = g.versions?.find(
								(v) => v.id === g.active_version_id,
							);
							return (
								<TableRow key={g.id}>
									<TableCell>
										<span className="font-medium">
											{g.display_name || g.name}
										</span>
										<span className="block text-content-secondary text-xs">
											{g.name}
										</span>
									</TableCell>
									<TableCell>{g.adapter_type}</TableCell>
									<TableCell>
										{activeVersion ? `v${activeVersion.version_number}` : "N/A"}
									</TableCell>
									<TableCell>
										<Badge variant={g.enabled ? "green" : "default"}>
											{g.enabled ? "Enabled" : "Disabled"}
										</Badge>
									</TableCell>
									<TableCell>
										<div className="flex flex-row justify-end gap-2">
											<Button
												size="sm"
												variant="outline"
												onClick={() => setEditing(g)}
											>
												Edit
											</Button>
											<Button
												size="sm"
												variant="outline"
												onClick={() => onToggle(g.id, !g.enabled)}
											>
												{g.enabled ? "Disable" : "Enable"}
											</Button>
											<Button
												size="sm"
												variant="outline"
												onClick={() => onDelete(g.id)}
											>
												Delete
											</Button>
										</div>
									</TableCell>
								</TableRow>
							);
						})
					)}
				</TableBody>
			</Table>
		</section>
	);
};

interface EditGuardrailDialogProps {
	guardrail: AIGatewayGuardrail | null;
	isEditing: boolean;
	editError: unknown;
	onClose: () => void;
	onEdit: (
		id: string,
		request: CreateAIGatewayGuardrailVersionRequest,
		onSuccess: () => void,
	) => void;
}

const EditGuardrailDialog: FC<EditGuardrailDialogProps> = ({
	guardrail,
	isEditing,
	editError,
	onClose,
	onEdit,
}) => (
	<Dialog
		open={guardrail !== null}
		onOpenChange={(next) => {
			if (!next) {
				onClose();
			}
		}}
	>
		<DialogContent>
			{guardrail && (
				<EditGuardrailForm
					key={guardrail.active_version_id ?? guardrail.id}
					guardrail={guardrail}
					isEditing={isEditing}
					editError={editError}
					onClose={onClose}
					onEdit={onEdit}
				/>
			)}
		</DialogContent>
	</Dialog>
);

interface EditGuardrailFormProps {
	guardrail: AIGatewayGuardrail;
	isEditing: boolean;
	editError: unknown;
	onClose: () => void;
	onEdit: (
		id: string,
		request: CreateAIGatewayGuardrailVersionRequest,
		onSuccess: () => void,
	) => void;
}

const EditGuardrailForm: FC<EditGuardrailFormProps> = ({
	guardrail,
	isEditing,
	editError,
	onClose,
	onEdit,
}) => {
	const configId = useId();
	const credentialId = useId();
	const activeVersion = guardrail.versions?.find(
		(v) => v.id === guardrail.active_version_id,
	);
	const promoteId = useId();
	const [config, setConfig] = useState(() =>
		JSON.stringify(activeVersion?.config ?? {}, null, 2),
	);
	const [credential, setCredential] = useState("");
	const [parseError, setParseError] = useState<string | null>(null);
	// Activating mints unpromoted drafts on each referencing pipeline by default
	// (the safe two-stage rollout); opting in promotes everywhere at once.
	const [promote, setPromote] = useState(false);

	const handleSubmit = (e: FormEvent) => {
		e.preventDefault();
		setParseError(null);
		let parsedConfig: CreateAIGatewayGuardrailVersionRequest["config"];
		try {
			parsedConfig = JSON.parse(config);
		} catch (err) {
			setParseError(
				err instanceof Error ? err.message : "config is not valid JSON",
			);
			return;
		}
		onEdit(
			guardrail.id,
			{
				config: parsedConfig,
				credential: credential || undefined,
				activate: true,
				promote,
			},
			onClose,
		);
	};

	return (
		<form onSubmit={handleSubmit}>
			<DialogHeader>
				<DialogTitle>
					Edit {guardrail.display_name || guardrail.name}
				</DialogTitle>
				<DialogDescription>
					Saving creates a new active version of this guardrail.
				</DialogDescription>
			</DialogHeader>

			<div className="flex flex-col gap-4 py-4">
				{editError ? <ErrorAlert error={editError} /> : null}
				<div className="flex flex-col gap-1">
					<Label htmlFor={configId}>Config (JSON)</Label>
					<textarea
						id={configId}
						value={config}
						onChange={(e) => setConfig(e.target.value)}
						rows={12}
						spellCheck={false}
						className="rounded border border-solid border-border bg-surface-secondary p-2 font-mono text-xs text-content-primary"
					/>
					{parseError ? (
						<span className="text-content-destructive text-xs">
							{parseError}
						</span>
					) : null}
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor={credentialId}>Credential (optional)</Label>
					<Input
						id={credentialId}
						type="password"
						value={credential}
						onChange={(e) => setCredential(e.target.value)}
						placeholder={
							activeVersion?.has_credential
								? "Re-enter the secret to keep it; leaving blank clears it"
								: "API key (stored encrypted); blank for adapters without a secret"
						}
					/>
				</div>
				<div className="flex items-start gap-2 rounded border border-solid border-border p-3">
					<Checkbox
						id={promoteId}
						checked={promote}
						onCheckedChange={(next) => setPromote(next === true)}
					/>
					<div className="flex flex-col gap-1">
						<Label htmlFor={promoteId} className="font-medium">
							Promote to live immediately
						</Label>
						<span className="text-xs text-content-secondary">
							When unchecked, the change is staged: each referencing pipeline
							gets a new unpromoted version that you promote separately. When
							checked, the change goes live on every referencing pipeline at
							once.
						</span>
					</div>
				</div>
			</div>

			<DialogFooter>
				<Button type="button" variant="outline" onClick={onClose}>
					Cancel
				</Button>
				<Button type="submit" disabled={isEditing}>
					<Spinner loading={isEditing} />
					Save new version
				</Button>
			</DialogFooter>
		</form>
	);
};

interface CreateGuardrailDialogProps {
	isCreating: boolean;
	createError: unknown;
	onCreate: (request: CreateAIGatewayGuardrailRequest) => void;
}

const CreateGuardrailDialog: FC<CreateGuardrailDialogProps> = ({
	isCreating,
	createError,
	onCreate,
}) => {
	const nameId = useId();
	const displayNameId = useId();
	const adapterId = useId();
	const configId = useId();
	const credentialId = useId();

	const [name, setName] = useState("");
	const [displayName, setDisplayName] = useState("");
	const [adapterType, setAdapterType] = useState("presidio");
	const [config, setConfig] = useState(PRESIDIO_CONFIG_EXAMPLE);
	const [credential, setCredential] = useState("");
	const [parseError, setParseError] = useState<string | null>(null);

	const handleSubmit = (e: FormEvent) => {
		e.preventDefault();
		setParseError(null);
		let parsedConfig: CreateAIGatewayGuardrailRequest["config"];
		try {
			parsedConfig = JSON.parse(config);
		} catch (err) {
			setParseError(
				err instanceof Error ? err.message : "config is not valid JSON",
			);
			return;
		}
		onCreate({
			name,
			display_name: displayName || undefined,
			adapter_type: adapterType,
			config: parsedConfig,
			credential: credential || undefined,
		});
	};

	return (
		<DialogContent>
			<form onSubmit={handleSubmit}>
				<DialogHeader>
					<DialogTitle>Create guardrail</DialogTitle>
					<DialogDescription>
						Define a networked guardrail and its first version.
					</DialogDescription>
				</DialogHeader>

				<div className="flex flex-col gap-4 py-4">
					{createError ? <ErrorAlert error={createError} /> : null}
					<div className="flex flex-col gap-1">
						<Label htmlFor={nameId}>Name</Label>
						<Input
							id={nameId}
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder="presidio-pii"
							required
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor={displayNameId}>Display name</Label>
						<Input
							id={displayNameId}
							value={displayName}
							onChange={(e) => setDisplayName(e.target.value)}
							placeholder="Presidio PII masking"
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor={adapterId}>Adapter type</Label>
						<Input
							id={adapterId}
							value={adapterType}
							onChange={(e) => setAdapterType(e.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor={configId}>Config (JSON)</Label>
						<textarea
							id={configId}
							value={config}
							onChange={(e) => setConfig(e.target.value)}
							rows={10}
							spellCheck={false}
							className="rounded border border-solid border-border bg-surface-secondary p-2 font-mono text-xs text-content-primary"
						/>
						{parseError ? (
							<span className="text-content-destructive text-xs">
								{parseError}
							</span>
						) : null}
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor={credentialId}>Credential (optional)</Label>
						<Input
							id={credentialId}
							type="password"
							value={credential}
							onChange={(e) => setCredential(e.target.value)}
							placeholder="API key (stored encrypted)"
						/>
					</div>
				</div>

				<DialogFooter>
					<Button type="submit" disabled={isCreating}>
						<Spinner loading={isCreating} />
						Create
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	);
};
