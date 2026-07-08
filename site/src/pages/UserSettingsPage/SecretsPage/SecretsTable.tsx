import { EllipsisVerticalIcon, PencilIcon, TrashIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type { UserSecret } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
};

export const SecretsTable: FC<SecretsTableProps> = ({
	secrets,
	isLoading,
	hasLoaded,
	isDeleting,
	onAddSecret,
	onEditSecret,
	onDeleteSecret,
}) => {
	const [secretToDelete, setSecretToDelete] = useState<UserSecret>();

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
						<TableHead className="w-[16%]">Name</TableHead>
						<TableHead className="w-[14%]">Environment variable</TableHead>
						<TableHead className="w-[18%]">File path</TableHead>
						<TableHead className="w-[11%]">Type</TableHead>
						<TableHead className="w-[23%]">Description</TableHead>
						<TableHead className="w-[12%]">Updated</TableHead>
						<TableHead className="w-[1%]" />
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
								<TableCell className="font-semibold text-content-primary">
									{secret.name}
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
								<TableCell>
									<OptionalSecretValue
										value={secret.description}
										fallback="No description"
									/>
								</TableCell>
								<TableCell data-pixel="ignore">
									{relativeTime(secret.updated_at)}
								</TableCell>
								<TableCell>
									<SecretRowActions
										secret={secret}
										onEditSecret={onEditSecret}
										onDeleteSecret={setSecretToDelete}
									/>
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
