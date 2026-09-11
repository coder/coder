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
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { shortRelativeTime } from "#/utils/time";
import { ChatProjectMemoryDialog } from "./ChatProjectMemoryDialog";

type ProjectMemorySectionProps = {
	readonly projectId: string;
};

/**
 * Lists the memories the agent has saved for a project. Rows are collapsed
 * to name and type by default; the body and actions only appear on expand.
 * Manual creation is available but intentionally understated: the agent is
 * the expected writer.
 */
export const ProjectMemorySection: FC<ProjectMemorySectionProps> = ({
	projectId,
}) => {
	const queryClient = useQueryClient();
	const memoriesQuery = useQuery(chatProjectMemories(projectId));
	const createMutation = useMutation(createChatProjectMemory(queryClient));
	const updateMutation = useMutation(updateChatProjectMemory(queryClient));
	const deleteMutation = useMutation(deleteChatProjectMemory(queryClient));
	const [editingMemory, setEditingMemory] = useState<
		TypesGen.ChatProjectMemory | null | undefined
	>(undefined);
	const [deletingMemory, setDeletingMemory] =
		useState<TypesGen.ChatProjectMemory | null>(null);
	const [expandedMemoryID, setExpandedMemoryID] = useState<string | null>(null);

	if (memoriesQuery.isLoading) {
		return <Skeleton className="mt-10 h-40 w-full" />;
	}
	if (memoriesQuery.error) {
		return <ErrorAlert error={memoriesQuery.error} className="mt-10" />;
	}
	const memories = memoriesQuery.data ?? [];

	return (
		<section className="mt-10 border-t border-border-default pt-8">
			<div className="flex items-start justify-between gap-4">
				<div>
					<h2 className="m-0 text-lg font-semibold text-content-primary">
						Memory
					</h2>
					<p className="mb-0 mt-1 text-sm text-content-secondary">
						Facts the agent saved while working in this project. Every chat in
						the project can read them.
					</p>
				</div>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Add memory"
					className="text-content-secondary"
					onClick={() => setEditingMemory(null)}
				>
					<PlusIcon />
				</Button>
			</div>
			{memories.length === 0 ? (
				<p className="mt-6 text-sm text-content-secondary">
					No memories yet. The agent saves them as it learns durable facts about
					this project.
				</p>
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
									<Badge size="xs" variant="default">
										{memory.type}
									</Badge>
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
			<ChatProjectMemoryDialog
				open={editingMemory !== undefined}
				memory={editingMemory}
				onOpenChange={(open) => {
					if (!open) setEditingMemory(undefined);
				}}
				onSubmit={async (request) => {
					if (editingMemory) {
						await updateMutation.mutateAsync({
							projectId,
							memoryId: editingMemory.id,
							request,
						});
						return;
					}
					await createMutation.mutateAsync({ projectId, request });
				}}
			/>
			<DeleteDialog
				isOpen={deletingMemory !== null}
				onCancel={() => setDeletingMemory(null)}
				onConfirm={() => {
					if (deletingMemory) {
						deleteMutation.mutate(
							{ projectId, memoryId: deletingMemory.id },
							{ onSuccess: () => setDeletingMemory(null) },
						);
					}
				}}
				entity="memory"
				name={deletingMemory?.name ?? ""}
				confirmLoading={deleteMutation.isPending}
			/>
		</section>
	);
};
