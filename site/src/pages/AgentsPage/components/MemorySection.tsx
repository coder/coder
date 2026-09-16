import { cn } from "cn";
import { ChevronRightIcon, PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectMemories,
	createChatProjectMemory,
	deleteChatProjectMemory,
	updateChatProjectMemory,
} from "#/api/queries/chatProjectMemories";
import {
	chatUserMemories,
	createChatUserMemory,
	deleteChatUserMemory,
	updateChatUserMemory,
} from "#/api/queries/chatUserMemories";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { shortRelativeTime } from "#/utils/time";
import { MemoryDialog, type MemoryRequest } from "./MemoryDialog";

type MemoryScope =
	| { readonly kind: "project"; readonly projectId: string }
	| { readonly kind: "personal"; readonly organizationId: string };

type MemorySectionProps = {
	readonly scope: MemoryScope;
	readonly title?: string;
	readonly description?: string;
};

type Memory = TypesGen.ChatProjectMemory | TypesGen.ChatUserMemory;

const projectMemoryCopy = {
	title: "Memory",
	description:
		"Facts the agent saved while working in this project. Every chat in the project can read them.",
	empty:
		"No memories yet. The agent saves them as it learns durable facts about this project.",
};

const personalMemoryCopy = {
	title: "Memory",
	description:
		"Facts the agent saves from your chats that are not in a project. Every such chat you own in this organization can read them.",
	empty:
		"No memories yet. The agent saves them as it learns durable facts from your chats.",
};

export const MemorySection: FC<MemorySectionProps> = ({
	scope,
	title,
	description,
}) => {
	const queryClient = useQueryClient();
	const projectMemoriesQuery = useQuery({
		...chatProjectMemories(scope.kind === "project" ? scope.projectId : ""),
		enabled: scope.kind === "project",
	});
	const personalMemoriesQuery = useQuery({
		...chatUserMemories(scope.kind === "personal" ? scope.organizationId : ""),
		enabled: scope.kind === "personal",
	});
	const createProjectMutation = useMutation(
		createChatProjectMemory(queryClient),
	);
	const updateProjectMutation = useMutation(
		updateChatProjectMemory(queryClient),
	);
	const deleteProjectMutation = useMutation(
		deleteChatProjectMemory(queryClient),
	);
	const createPersonalMutation = useMutation(createChatUserMemory(queryClient));
	const updatePersonalMutation = useMutation(updateChatUserMemory(queryClient));
	const deletePersonalMutation = useMutation(deleteChatUserMemory(queryClient));
	const [editingMemory, setEditingMemory] = useState<Memory | null | undefined>(
		undefined,
	);
	const [deletingMemory, setDeletingMemory] = useState<Memory | null>(null);
	const [expandedMemoryID, setExpandedMemoryID] = useState<string | null>(null);

	const memories =
		scope.kind === "project"
			? (projectMemoriesQuery.data ?? [])
			: (personalMemoriesQuery.data ?? []);
	const memoriesError =
		scope.kind === "project"
			? projectMemoriesQuery.error
			: personalMemoriesQuery.error;
	const isLoadingMemories =
		scope.kind === "project"
			? projectMemoriesQuery.isLoading
			: personalMemoriesQuery.isLoading;
	const copy =
		scope.kind === "project" ? projectMemoryCopy : personalMemoryCopy;
	const resolvedTitle = title ?? copy.title;
	const resolvedDescription = description ?? copy.description;
	const isDeleting =
		deleteProjectMutation.isPending || deletePersonalMutation.isPending;
	const deleteError =
		scope.kind === "project"
			? deleteProjectMutation.error
			: deletePersonalMutation.error;

	const submitMemory = async (request: MemoryRequest) => {
		if (scope.kind === "project") {
			if (editingMemory) {
				await updateProjectMutation.mutateAsync({
					projectId: scope.projectId,
					memoryId: editingMemory.id,
					request,
				});
				return;
			}
			await createProjectMutation.mutateAsync({
				projectId: scope.projectId,
				request,
			});
			return;
		}

		if (editingMemory) {
			await updatePersonalMutation.mutateAsync({
				memoryId: editingMemory.id,
				request,
			});
			return;
		}
		await createPersonalMutation.mutateAsync({
			organization_id: scope.organizationId,
			...request,
		});
	};

	const deleteMemory = () => {
		if (!deletingMemory) {
			return;
		}
		if (scope.kind === "project") {
			deleteProjectMutation.mutate(
				{ projectId: scope.projectId, memoryId: deletingMemory.id },
				{ onSuccess: () => setDeletingMemory(null) },
			);
			return;
		}
		deletePersonalMutation.mutate(deletingMemory.id, {
			onSuccess: () => setDeletingMemory(null),
		});
	};

	if (isLoadingMemories) {
		return <Skeleton className="mt-10 h-40 w-full" />;
	}
	if (memoriesError) {
		return <ErrorAlert error={memoriesError} className="mt-10" />;
	}

	return (
		<section className="mt-10 border-t border-border-default pt-8">
			<div className="flex items-start justify-between gap-4">
				<div>
					<h2 className="m-0 text-lg font-semibold text-content-primary">
						{resolvedTitle}
					</h2>
					{resolvedDescription && (
						<p className="mb-0 mt-1 text-sm text-content-secondary">
							{resolvedDescription}
						</p>
					)}
				</div>
				<Button onClick={() => setEditingMemory(null)}>
					<PlusIcon />
					Add memory
				</Button>
			</div>
			{deletingMemory && deleteError ? (
				<ErrorAlert error={deleteError} className="mt-4" />
			) : null}
			{memories.length === 0 ? (
				<p className="mt-6 text-sm text-content-secondary">{copy.empty}</p>
			) : (
				<ul className="mt-4 list-none divide-y divide-border-default rounded-md border border-border-default p-0">
					{memories.map((memory) => {
						const expanded = expandedMemoryID === memory.id;
						return (
							<li key={memory.id}>
								<button
									type="button"
									aria-expanded={expanded}
									className={cn(
										"flex w-full items-center gap-2 border-0 bg-transparent px-3 py-2 text-left",
										"text-content-primary hover:bg-surface-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
									)}
									onClick={() =>
										setExpandedMemoryID(expanded ? null : memory.id)
									}
								>
									<ChevronRightIcon
										className={cn(
											"size-4 shrink-0 text-content-secondary transition-transform",
											expanded && "rotate-90",
										)}
									/>
									<span className="min-w-0 flex-1 truncate font-mono text-sm">
										{memory.name}
									</span>
								</button>
								{expanded && (
									<div className="space-y-3 px-3 pb-3 pl-9">
										<p className="mb-0 text-sm text-content-secondary">
											{memory.description}
										</p>
										<MemoizedMarkdown>{memory.body}</MemoizedMarkdown>
										<div className="flex items-center justify-between gap-2 text-xs text-content-secondary">
											<span>
												Updated {shortRelativeTime(memory.updated_at)} by{" "}
												{memory.created_by_username || "Unknown"}
											</span>
											<span className="flex gap-1">
												<Button
													variant="subtle"
													size="sm"
													onClick={() => setEditingMemory(memory)}
												>
													Edit
												</Button>
												<Button
													variant="subtle"
													size="sm"
													className="text-content-destructive"
													onClick={() => setDeletingMemory(memory)}
												>
													Delete
												</Button>
											</span>
										</div>
									</div>
								)}
							</li>
						);
					})}
				</ul>
			)}
			<MemoryDialog
				key={editingMemory?.id ?? (editingMemory === null ? "new" : "closed")}
				open={editingMemory !== undefined}
				memory={editingMemory}
				onOpenChange={(open) => {
					if (!open) setEditingMemory(undefined);
				}}
				onSubmit={submitMemory}
			/>
			<DeleteDialog
				isOpen={deletingMemory !== null}
				onCancel={() => setDeletingMemory(null)}
				onConfirm={deleteMemory}
				entity="memory"
				name={deletingMemory?.name ?? ""}
				confirmLoading={isDeleting}
			/>
		</section>
	);
};
