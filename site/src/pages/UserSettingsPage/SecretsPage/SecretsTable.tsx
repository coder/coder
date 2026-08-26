import { EllipsisVerticalIcon, PencilIcon, TrashIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type { UserSecret } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Switch } from "#/components/Switch/Switch";
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { relativeTime } from "#/utils/time";

type SecretsTableProps = {
	secrets?: readonly UserSecret[];
	isLoading: boolean;
	hasLoaded: boolean;
	isDeleting: boolean;
	onAddSecret: (returnFocusElement?: HTMLElement | null) => void;
	onEditSecret: (
		secret: UserSecret,
		returnFocusElement?: HTMLElement | null,
	) => void;
	onDeleteSecret: (secret: UserSecret) => Promise<void> | void;
	onToggleEnabled: (
		secret: UserSecret,
		enabled: boolean,
	) => Promise<void> | void;
};

export const SecretsTable: FC<SecretsTableProps> = ({
	secrets,
	isLoading,
	hasLoaded,
	isDeleting,
	onAddSecret,
	onEditSecret,
	onDeleteSecret,
	onToggleEnabled,
}) => {
	const [secretToDelete, setSecretToDelete] = useState<UserSecret>();
	const [togglingSecretId, setTogglingSecretId] = useState<string | null>(null);

	const handleToggle = (secret: UserSecret, enabled: boolean) => {
		setTogglingSecretId(secret.id);
		void Promise.resolve()
			.then(() => onToggleEnabled(secret, enabled))
			.catch(() => {
				// onToggleEnabled reports failures with a toast before rejecting.
				// Swallow the rejection here to avoid an unhandled promise rejection warning.
			})
			.finally(() => {
				setTogglingSecretId((current) =>
					current === secret.id ? null : current,
				);
			});
	};

	return (
		<>
			<DeleteSecretDialog
				secret={secretToDelete}
				isDeleting={isDeleting}
				onCancel={() => setSecretToDelete(undefined)}
				onConfirm={(secret) => {
					void Promise.resolve()
						.then(() => onDeleteSecret(secret))
						.then(() => {
							setSecretToDelete(undefined);
						})
						.catch(() => {
							// onDeleteSecret reports failures with a toast before rejecting.
							// Swallow the rejection here to avoid an unhandled promise rejection warning.
						});
				}}
			/>

			<Table aria-label="User secrets">
				<TableHeader>
					<TableRow>
						<TableHead className="w-9"></TableHead>
						<TableHead>Name</TableHead>
						<TableHead>Env var</TableHead>
						<TableHead className="whitespace-nowrap">File path</TableHead>
						<TableHead>Type</TableHead>
						<TableHead className="w-full">Description</TableHead>
						<TableHead>Updated</TableHead>
						<TableHead></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading && <TableLoader />}
					{hasLoaded && !isLoading && (!secrets || secrets.length === 0) && (
						<TableEmpty
							message="No secrets yet"
							description="Create a secret to inject it into workspaces you own."
							cta={
								<Button onClick={(event) => onAddSecret(event.currentTarget)}>
									Add secret
								</Button>
							}
						/>
					)}
					{!isLoading &&
						secrets?.map((secret) => (
							<TableRow key={secret.id}>
								<TableCell>
									<EnabledToggle
										secret={secret}
										isPending={togglingSecretId === secret.id}
										onToggle={handleToggle}
									/>
								</TableCell>
								<TableCell className="font-semibold text-content-primary">
									<span>{secret.name}</span>
								</TableCell>
								<TableCell>
									<OptionalSecretValue value={secret.env_name} />
								</TableCell>
								<TableCell>
									<OptionalSecretValue value={secret.file_path} />
								</TableCell>
								<TableCell>
									<SecretTypeBadge secret={secret} />
								</TableCell>
								<TableCell className="max-w-0">
									{secret.description ? (
										<span className="block truncate" title={secret.description}>
											{secret.description}
										</span>
									) : (
										<span className="text-content-disabled">
											No description
										</span>
									)}
								</TableCell>
								<TableCell data-pixel="ignore" className="whitespace-nowrap">
									{relativeTime(secret.updated_at)}
								</TableCell>
								<TableCell>
									<div className="flex justify-end flex-1">
										<SecretRowActions
											secret={secret}
											onEditSecret={onEditSecret}
											onDeleteSecret={setSecretToDelete}
										/>
									</div>
								</TableCell>
							</TableRow>
						))}
				</TableBody>
			</Table>
		</>
	);
};

const OptionalSecretValue: FC<{ value?: string; fallback?: string }> = ({
	value,
	fallback = "Not set",
}) => {
	if (value) {
		return value;
	}

	return <span className="text-content-disabled">{fallback}</span>;
};

const SecretTypeBadge: FC<{ secret: UserSecret }> = ({ secret }) => {
	const hasEnv = Boolean(secret.env_name);
	const hasFile = Boolean(secret.file_path);

	if (hasEnv && hasFile) {
		return <Badge>env var + file</Badge>;
	}

	if (hasEnv) {
		return <Badge>env var</Badge>;
	}

	if (hasFile) {
		return <Badge>file</Badge>;
	}

	return <Badge>not injected</Badge>;
};

type EnabledToggleProps = {
	secret: UserSecret;
	isPending: boolean;
	onToggle: (secret: UserSecret, enabled: boolean) => void;
};

const EnabledToggle: FC<EnabledToggleProps> = ({
	secret,
	isPending,
	onToggle,
}) => {
	const hasTarget = Boolean(secret.env_name) || Boolean(secret.file_path);
	// An enabled secret must have at least one injection target. Prevent
	// enabling a target-less secret; the user must add a target first.
	const cannotEnable = !secret.enabled && !hasTarget;

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				{/*
				 * Wrap the disabled Switch in a focusable span so the
				 * tooltip can be triggered by keyboard and pointer.
				 * oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- needed to
				 * surface the tooltip on a disabled control via keyboard focus.
				 */}
				<span tabIndex={0} className="inline-flex">
					<Switch
						aria-label={`Toggle secret ${secret.name}`}
						checked={secret.enabled}
						disabled={isPending || cannotEnable}
						onCheckedChange={(checked) => onToggle(secret, checked)}
					/>
				</span>
			</TooltipTrigger>
			{cannotEnable && (
				<TooltipContent side="top">
					Add an environment variable or file path before enabling this secret.
				</TooltipContent>
			)}
		</Tooltip>
	);
};

type SecretRowActionsProps = {
	secret: UserSecret;
	onEditSecret: (
		secret: UserSecret,
		returnFocusElement?: HTMLElement | null,
	) => void;
	onDeleteSecret: (secret: UserSecret) => void;
};

const SecretRowActions: FC<SecretRowActionsProps> = ({
	secret,
	onEditSecret,
	onDeleteSecret,
}) => {
	const label = `Open secret actions for ${secret.name}`;
	const triggerRef = useRef<HTMLButtonElement>(null);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					ref={triggerRef}
					size="icon"
					variant="subtle"
					aria-label={label}
				>
					<EllipsisVerticalIcon aria-hidden="true" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem
					onSelect={() => onEditSecret(secret, triggerRef.current)}
				>
					<PencilIcon className="size-icon-xs" />
					Edit secret
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<DropdownMenuItem
					className="text-content-destructive focus:text-content-destructive"
					onSelect={() => onDeleteSecret(secret)}
				>
					<TrashIcon className="size-icon-xs" />
					Delete
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

type DeleteSecretDialogProps = {
	secret?: UserSecret;
	isDeleting: boolean;
	onCancel: () => void;
	onConfirm: (secret: UserSecret) => void;
};

const DeleteSecretDialog: FC<DeleteSecretDialogProps> = ({
	secret,
	isDeleting,
	onCancel,
	onConfirm,
}) => {
	return (
		<ConfirmDialog
			type="delete"
			open={Boolean(secret)}
			confirmLoading={isDeleting}
			title="Delete secret"
			description={
				<p>
					Deleting <strong>{secret?.name}</strong> is irreversible. Workspaces
					that depend on this secret will no longer receive it on future starts.
				</p>
			}
			onClose={() => {
				if (!isDeleting) {
					onCancel();
				}
			}}
			onConfirm={() => {
				if (secret) {
					onConfirm(secret);
				}
			}}
		/>
	);
};
