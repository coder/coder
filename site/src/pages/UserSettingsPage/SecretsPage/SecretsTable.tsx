import { EllipsisVerticalIcon, PencilIcon, TrashIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { UserSecret } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
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
	onAddSecret: () => void;
	onEditSecret: (secret: UserSecret) => void;
	onDeleteSecret: (secret: UserSecret) => void;
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
					onDeleteSecret(secret);
					setSecretToDelete(undefined);
				}}
			/>

			<Table aria-label="User secrets">
				<TableHeader>
					<TableRow>
						<TableHead className="w-[16%]">Name</TableHead>
						<TableHead className="w-[14%]">Env var</TableHead>
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
							cta={<Button onClick={onAddSecret}>Add secret</Button>}
						/>
					)}
					{!isLoading &&
						secrets?.map((secret) => (
							<TableRow key={secret.id}>
								<TableCell className="font-semibold text-content-primary">
									{secret.name}
								</TableCell>
								<TableCell>{secret.env_name || "Not set"}</TableCell>
								<TableCell>{secret.file_path || "Not set"}</TableCell>
								<TableCell>
									<SecretTypeBadge secret={secret} />
								</TableCell>
								<TableCell>{secret.description || "No description"}</TableCell>
								<TableCell data-chromatic="ignore">
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

const SecretTypeBadge: FC<{ secret: UserSecret }> = ({ secret }) => {
	const hasEnv = secret.env_name !== "";
	const hasFile = secret.file_path !== "";
	const label =
		hasEnv && hasFile
			? ".env + file"
			: hasEnv
				? ".env"
				: hasFile
					? "file"
					: "not injected";

	return <Badge size="sm">{label}</Badge>;
};

type SecretRowActionsProps = {
	secret: UserSecret;
	onEditSecret: (secret: UserSecret) => void;
	onDeleteSecret: (secret: UserSecret) => void;
};

const SecretRowActions: FC<SecretRowActionsProps> = ({
	secret,
	onEditSecret,
	onDeleteSecret,
}) => {
	const label = `Open secret actions for ${secret.name}`;

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button size="icon" variant="subtle" aria-label={label}>
					<EllipsisVerticalIcon aria-hidden="true" />
				</Button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end">
				<DropdownMenuItem onSelect={() => onEditSecret(secret)}>
					<PencilIcon className="size-icon-xs" />
					Edit secret...
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<DropdownMenuItem
					className="text-content-destructive focus:text-content-destructive"
					onSelect={() => onDeleteSecret(secret)}
				>
					<TrashIcon className="size-icon-xs" />
					Delete...
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
		<Dialog open={Boolean(secret)} onOpenChange={(open) => !open && onCancel()}>
			<DialogContent variant="destructive" aria-describedby={undefined}>
				<DialogHeader>
					<DialogTitle>Delete secret</DialogTitle>
				</DialogHeader>
				<p className="m-0 text-sm leading-relaxed text-content-secondary">
					Deleting{" "}
					<strong className="text-content-primary">{secret?.name}</strong> is
					irreversible. Workspaces that depend on this secret will no longer
					receive it on future starts.
				</p>
				<DialogFooter>
					<DialogActions
						cancelText="Cancel"
						confirmText="Delete"
						confirmVariant="destructive"
						confirmLoading={isDeleting}
						onCancel={onCancel}
						onConfirm={() => {
							if (secret) {
								onConfirm(secret);
							}
						}}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
