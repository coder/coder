import { API } from "api/api";
import { workspaceBuildParameters } from "api/queries/workspaceBuilds";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import {
	AITaskPromptParameterName,
	type Task,
	type Workspace,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Spinner } from "components/Spinner/Spinner";
import { Textarea } from "components/Textarea/Textarea";
import type { FC } from "react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";

type ModifyPromptDialogProps = {
	task: Task;
	workspace: Workspace;
	open: boolean;
	onOpenChange: (open: boolean) => void;
};

export const ModifyPromptDialog: FC<ModifyPromptDialogProps> = ({
	task,
	workspace,
	open,
	onOpenChange,
}) => {
	const [prompt, setPrompt] = useState(task.initial_prompt);
	const queryClient = useQueryClient();

	const buildParametersQuery = useQuery(
		workspaceBuildParameters(workspace.latest_build.id),
	);

	const updatePromptMutation = useMutation({
		mutationFn: async () => {
			const currentBuild = await API.getWorkspaceBuildByNumber(
				workspace.owner_name,
				workspace.name,
				workspace.latest_build.build_number,
			);

			if (currentBuild.status !== "stopped") {
				await API.cancelWorkspaceBuild(workspace.latest_build.id);
				try {
					await API.waitForBuild(currentBuild);
				} catch (error: unknown) {
					if (error && typeof error === "object" && "status" in error) {
						// `waitForBuild` throws when a build "fails", which it does
						// when it is canceled.
					} else {
						throw error;
					}
				}

				const stopBuild = await API.stopWorkspace(workspace.id);
				await API.waitForBuild(stopBuild);
			}

			await API.updateTaskInput(task.owner_name, task.id, prompt);

			// Start a new build with the updated prompt
			await API.startWorkspace(
				workspace.id,
				task.template_version_id,
				undefined,
				buildParametersQuery.data?.map((parameter) =>
					parameter.name === AITaskPromptParameterName
						? { ...parameter, value: prompt }
						: parameter,
				),
			);
		},
		onSuccess: () => {
			queryClient.invalidateQueries({
				queryKey: ["tasks", task.owner_name, task.id],
			});
			queryClient.invalidateQueries({
				queryKey: workspaceByOwnerAndNameKey(
					workspace.owner_name,
					workspace.name,
				),
			});

			onOpenChange(false);
		},
	});

	const workspaceBuildRunning = workspace.latest_build.status === "running";
	const promptModified = prompt !== task.initial_prompt;
	const promptCanBeModified =
		prompt.length !== 0 && promptModified && !workspaceBuildRunning;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-2xl">
				<DialogHeader>
					<DialogTitle>Modify Task Prompt</DialogTitle>
					<DialogDescription>
						Modifying the prompt will cancel the current workspace build and
						restart it with the updated prompt. This is only possible while the
						build is pending or starting.
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					{updatePromptMutation.error && (
						<ErrorAlert error={updatePromptMutation.error} />
					)}
					{workspaceBuildRunning && (
						<ErrorAlert error={"Cannot modify the prompt of a running task"} />
					)}

					<div>
						<label
							htmlFor="prompt"
							className="block text-sm font-medium text-content-primary mb-2"
						>
							Prompt
						</label>
						<Textarea
							id="prompt"
							value={prompt}
							onChange={(e) => setPrompt(e.target.value)}
							rows={10}
							disabled={updatePromptMutation.isPending || workspaceBuildRunning}
							className="w-full"
							placeholder="Enter your task prompt..."
						/>
					</div>
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						onClick={() => onOpenChange(false)}
						disabled={updatePromptMutation.isPending}
					>
						Cancel
					</Button>
					<Button
						onClick={() => updatePromptMutation.mutate()}
						disabled={
							!promptCanBeModified ||
							updatePromptMutation.isPending ||
							buildParametersQuery.isLoading
						}
					>
						<Spinner loading={updatePromptMutation.isPending} />
						Update and Restart Build
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
