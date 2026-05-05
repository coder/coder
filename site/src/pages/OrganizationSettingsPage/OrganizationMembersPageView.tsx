import { TriangleAlertIcon, UserPlusIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { User } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import type { useFilter } from "#/components/Filter/Filter";
import { UsersFilter } from "#/components/Filter/UsersFilter";
import { MultiUserSelect } from "#/components/MultiUserSelect/MultiUserSelect";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import type { PaginationResultInfo } from "#/hooks/usePaginatedQuery";
import {
	OrganizationMembersTable,
	type OrganizationMembersTableProps,
} from "./OrganizationMembersTable";

type OrganizationMembersPageViewProps = OrganizationMembersTableProps & {
	error: unknown;
	filterProps: { filter: ReturnType<typeof useFilter> };
	membersQuery: PaginationResultInfo & {
		isPlaceholderData: boolean;
	};
	addMembers: (users: User[]) => Promise<void>;
	canViewMembers?: boolean;
};

export const OrganizationMembersPageView: React.FC<
	OrganizationMembersPageViewProps
> = ({
	error,
	filterProps,
	membersQuery,
	canViewMembers,
	addMembers,
	...props
}) => {
	const { canEditMembers } = props;

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Members</SettingsHeaderTitle>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				{Boolean(error) && <ErrorAlert error={error} />}

				<div className="flex flex-row justify-between">
					<UsersFilter {...filterProps} />
					{canEditMembers && <AddUsersDialog onSubmit={addMembers} />}
				</div>
				{!canViewMembers && (
					<div className="flex flex-row text-content-warning gap-2 items-center text-sm font-medium">
						<TriangleAlertIcon className="size-icon-sm" />
						<p>
							You do not have permission to view members other than yourself.
						</p>
					</div>
				)}
				<PaginationContainer query={membersQuery} paginationUnitLabel="members">
					<OrganizationMembersTable {...props} />
				</PaginationContainer>
			</div>
		</div>
	);
};

interface AddUsersDialogProps {
	onSubmit: (users: User[]) => Promise<void>;
}

const AddUsersDialog: React.FC<AddUsersDialogProps> = ({ onSubmit }) => {
	const [addUserDialogOpen, setAddUserDialogOpen] = useState(false);
	const [submitting, setSubmitting] = useState(false);
	const [filter, setFilter] = useState("");
	const [selected, setSelected] = useState<User[]>([]);
	const closeDialog = () => {
		setAddUserDialogOpen(false);
		setFilter("");
		setSelected([]);
	};

	return (
		<>
			<Button size="lg" onClick={() => setAddUserDialogOpen(true)}>
				<UserPlusIcon />
				Add users
			</Button>
			<Dialog
				open={addUserDialogOpen}
				onOpenChange={(open) => {
					if (!open) {
						closeDialog();
					}
				}}
			>
				<DialogContent
					data-testid="dialog"
					className="max-w-md gap-4 border-border-default bg-surface-primary p-8 text-content-primary"
				>
					<DialogTitle className="font-semibold text-content-primary">
						Add user(s)
					</DialogTitle>
					<MultiUserSelect
						filter={filter}
						setFilter={setFilter}
						onChange={(user, checked) => {
							if (checked) {
								setSelected([...selected, user]);
							} else {
								setSelected(selected.filter((s) => s.id !== user.id));
							}
						}}
						selected={selected}
					/>
					<DialogFooter className="mt-4 flex-row justify-end gap-3">
						<Button
							variant="outline"
							onClick={closeDialog}
							disabled={submitting}
						>
							Cancel
						</Button>
						<Button
							disabled={submitting || selected.length === 0}
							onClick={async () => {
								try {
									setSubmitting(true);
									await onSubmit(selected);
									closeDialog();
								} catch (error) {
									toast.error(
										getErrorMessage(error, "Failed to add members."),
										{
											description: getErrorDetail(error),
										},
									);
								} finally {
									setSubmitting(false);
								}
							}}
						>
							<Spinner loading={submitting} />
							Add users
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
};
