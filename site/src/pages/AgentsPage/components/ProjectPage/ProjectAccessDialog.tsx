import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { RetryButton } from "../RetryButton";
import { chatProjectRoleLabel, type ProjectAccessEntry } from "./projectAccess";

type ProjectAccessDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** `undefined` while the access list loads or after loading fails. */
	readonly entries: readonly ProjectAccessEntry[] | undefined;
	readonly error: unknown;
	readonly isRetrying: boolean;
	readonly onRetry: () => void;
	readonly onCloseAutoFocus: (event: Event) => void;
};

/** Lists who can use a project and their role; onCloseAutoFocus restores focus. */
export const ProjectAccessDialog: React.FC<ProjectAccessDialogProps> = ({
	open,
	onOpenChange,
	entries,
	error,
	isRetrying,
	onRetry,
	onCloseAutoFocus,
}) => (
	<Dialog open={open} onOpenChange={onOpenChange}>
		<DialogContent
			className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
			onCloseAutoFocus={onCloseAutoFocus}
		>
			<DialogHeader className="px-6 pt-6">
				<DialogTitle>View access</DialogTitle>
			</DialogHeader>
			<div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-4">
				{Boolean(error) && (
					<ErrorAlert
						error={error}
						showDebugDetail={false}
						actions={
							<RetryButton
								size="sm"
								variant="outline"
								isRetrying={isRetrying}
								onRetry={onRetry}
							/>
						}
					/>
				)}
				{(entries || !error) && (
					<Table aria-label="Project access">
						<TableHeader>
							<TableRow>
								<TableHead className="w-full">User or group</TableHead>
								<TableHead>Role</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{entries ? (
								entries.map((entry) => (
									<AccessRow key={`${entry.kind}:${entry.id}`} entry={entry} />
								))
							) : (
								<TableLoader />
							)}
						</TableBody>
					</Table>
				)}
				<DialogDescription className="font-normal">
					Chats in this project start private. Their creators can still share
					them individually.
				</DialogDescription>
			</div>
			<DialogFooter className="border-t border-border px-6 py-4">
				<Button variant="outline" onClick={() => onOpenChange(false)}>
					Close
				</Button>
			</DialogFooter>
		</DialogContent>
	</Dialog>
);

type AccessRowProps = {
	readonly entry: ProjectAccessEntry;
};

const AccessRow: React.FC<AccessRowProps> = ({ entry }) => (
	<TableRow>
		<TableCell>
			<AvatarData
				title={entry.name}
				subtitle={entry.subtitle}
				src={entry.avatarUrl}
			/>
		</TableCell>
		<TableCell>
			{entry.kind === "owner" ? (
				<Badge variant="info">Owner</Badge>
			) : (
				<Badge>
					{entry.role === "" ? "Unknown" : chatProjectRoleLabel[entry.role]}
				</Badge>
			)}
		</TableCell>
	</TableRow>
);
