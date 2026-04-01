import { API } from "api/api";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Skeleton } from "components/Skeleton/Skeleton";
import { Spinner } from "components/Spinner/Spinner";
import type { FC } from "react";
import { useQuery } from "react-query";

interface DisableWorkspaceSharingDialogProps {
	isOpen: boolean;
	organizationId: string;
	onConfirm: () => void;
	onCancel: () => void;
	isLoading?: boolean;
}

export const DisableWorkspaceSharingDialog: FC<
	DisableWorkspaceSharingDialogProps
> = ({ isOpen, organizationId, onConfirm, onCancel, isLoading }) => {
	// Fetch the count of shared workspaces in this organization
	const sharedWorkspacesQuery = useQuery({
		queryKey: ["workspaces", organizationId, "shared", "count"],
		queryFn: async () => {
			const response = await API.getWorkspaces({
				q: `organization:${organizationId} shared:true`,
				limit: 0, // Avoid fetching workspaces as we only need the count
			});
			return response.count;
		},
		enabled: isOpen,
	});

	const sharedCount = sharedWorkspacesQuery.data ?? 0;
	const isLoadingCount = sharedWorkspacesQuery.isLoading;

	return (
		<Dialog open={isOpen} onOpenChange={(open) => !open && onCancel()}>
			<DialogContent variant="destructive" className="max-w-xl">
				<DialogHeader>
					<DialogTitle>Disable workspace sharing</DialogTitle>
					<DialogDescription asChild>
						<div className="flex flex-col gap-4">
							<p>
								Disabling workspace sharing will{" "}
								<strong className="text-content-primary">
									immediately remove
								</strong>{" "}
								all existing workspace sharing permissions for all users in this
								organization.
							</p>
							{isLoadingCount ? (
								<Skeleton className="h-6 w-4/5" />
							) : sharedCount > 0 ? (
								<p className="text-content-danger font-medium m-0">
									This action will affect{" "}
									<strong className="text-content-primary">
										{sharedCount} workspace{sharedCount !== 1 ? "s" : ""}
									</strong>{" "}
									that {sharedCount !== 1 ? "are" : "is"} currently shared.
								</p>
							) : (
								<p className="text-content-secondary m-0">
									No workspaces are currently shared in this organization.
								</p>
							)}
							<p>
								Re-enabling workspace sharing will{" "}
								<strong className="text-content-primary">not restore</strong>{" "}
								these permissions.
							</p>
						</div>
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button variant="outline" onClick={onCancel} disabled={isLoading}>
						Cancel
					</Button>
					<Button
						variant="destructive"
						onClick={onConfirm}
						disabled={isLoading}
					>
						<Spinner loading={isLoading} />
						Disable sharing
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
