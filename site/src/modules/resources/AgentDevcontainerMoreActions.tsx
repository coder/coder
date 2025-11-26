import { API } from "api/api";
import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
	WorkspaceAgentListContainersResponse,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EllipsisVertical } from "lucide-react";
import { type FC, useId, useState } from "react";
import { useMutation, useQueryClient } from "react-query";

type AgentDevcontainerMoreActionsProps = {
	parentAgent: WorkspaceAgent;
	devcontainer: WorkspaceAgentDevcontainer;
};

export const AgentDevcontainerMoreActions: FC<
	AgentDevcontainerMoreActionsProps
> = ({ parentAgent, devcontainer }) => {
	const queryClient = useQueryClient();
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const [open, setOpen] = useState(false);
	const menuContentId = useId();

	const deleteDevContainerMutation = useMutation({
		mutationFn: async () => {
			await API.deleteDevContainer({
				parentAgentId: parentAgent.id,
				devcontainerId: devcontainer.id,
			});
		},
		onMutate: async () => {
			await queryClient.cancelQueries({
				queryKey: ["agents", parentAgent.id, "containers"],
			});

			const previousData = queryClient.getQueryData([
				"agents",
				parentAgent.id,
				"containers",
			]);

			queryClient.setQueryData(
				["agents", parentAgent.id, "containers"],
				(oldData?: WorkspaceAgentListContainersResponse) => {
					if (!oldData?.devcontainers) return oldData;
					return {
						...oldData,
						devcontainers: oldData.devcontainers.map((dc) => {
							if (dc.id === devcontainer.id) {
								return {
									...dc,
									status: "stopping",
									container: undefined,
								};
							}
							return dc;
						}),
					};
				},
			);

			return { previousData };
		},
		onError: (_, __, context) => {
			if (context?.previousData) {
				queryClient.setQueryData(
					["agents", parentAgent.id, "containers"],
					context.previousData,
				);
			}
		},
	});

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<Button size="icon-lg" variant="subtle" aria-controls={menuContentId}>
					<EllipsisVertical aria-hidden="true" />
					<span className="sr-only">Dev Container actions</span>
				</Button>
			</DropdownMenuTrigger>

			<DropdownMenuContent id={menuContentId} align="end">
				<DropdownMenuItem
					className="text-content-destructive focus:text-content-destructive"
					onClick={() => {
						setIsConfirmingDelete(true);
					}}
				>
					Delete&hellip;
				</DropdownMenuItem>
			</DropdownMenuContent>

			<DevcontainerDeleteDialog
				isOpen={isConfirmingDelete}
				onCancel={() => setIsConfirmingDelete(false)}
				onConfirm={() => {
					deleteDevContainerMutation.mutate();
					setIsConfirmingDelete(false);
				}}
			/>
		</DropdownMenu>
	);
};

type DevcontainerDeleteDialogProps = {
	isOpen: boolean;
	onCancel: () => void;
	onConfirm: () => void;
};

const DevcontainerDeleteDialog: FC<DevcontainerDeleteDialogProps> = ({
	isOpen,
	onCancel,
	onConfirm,
}) => {
	return (
		<ConfirmDialog
			type="delete"
			open={isOpen}
			title="Delete Dev Container"
			onConfirm={onConfirm}
			onClose={onCancel}
			description={
				<p>
					Are you sure you want to delete this Dev Container? Any unsaved work
					will be lost.
				</p>
			}
		/>
	);
};
