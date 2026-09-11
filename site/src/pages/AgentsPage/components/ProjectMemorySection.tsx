import { ChevronDownIcon, EllipsisVerticalIcon, PlusIcon } from "lucide-react";
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
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { shortRelativeTime } from "#/utils/time";
import { ChatProjectMemoryDialog } from "./ChatProjectMemoryDialog";

const memoryTypes = ["user", "feedback", "project", "reference"] as const;

type ProjectMemorySectionProps = {
	readonly projectId: string;
	readonly state?: Readonly<{
		isLoading?: boolean;
		error?: unknown;
		memories?: readonly TypesGen.ChatProjectMemory[];
	}>;
};

export const ProjectMemorySection: FC<ProjectMemorySectionProps> = ({
	projectId,
	state,
}) => {
	const queryClient = useQueryClient();
	const memoriesQuery = useQuery({
		...chatProjectMemories(projectId),
		enabled: state === undefined,
	});
	const createMutation = useMutation(createChatProjectMemory(queryClient));
	const updateMutation = useMutation(updateChatProjectMemory(queryClient));
	const deleteMutation = useMutation(deleteChatProjectMemory(queryClient));
	const [editingMemory, setEditingMemory] = useState<
		TypesGen.ChatProjectMemory | null | undefined
	>(undefined);
	const [deletingMemory, setDeletingMemory] =
		useState<TypesGen.ChatProjectMemory | null>(null);
	const [expandedMemoryIDs, setExpandedMemoryIDs] = useState<Set<string>>(
		new Set(),
	);

	const isLoading = state?.isLoading ?? memoriesQuery.isLoading;
	const error = state?.error ?? memoriesQuery.error;
	const memories = state?.memories ?? memoriesQuery.data ?? [];

	if (isLoading) {
		return <Skeleton className="mt-10 h-40 w-full" />;
	}
	if (error) {
		return <ErrorAlert error={error} className="mt-10" />;
	}

	return (
		<section className="mt-10 border-t border-border-default pt-8">
			<div className="flex items-start justify-between gap-4">
				<div>
					<h2 className="m-0 text-lg font-semibold text-content-primary">
						Memory
					</h2>
					<p className="mb-0 mt-1 text-sm text-content-secondary">
						Memories the agent saves while working in this project. Every chat
						in the project sees this list.
					</p>
				</div>
				<Button onClick={() => setEditingMemory(null)}>
					<PlusIcon />
					Add memory
				</Button>
			</div>
			{memories.length === 0 ? (
				<p className="mt-6 text-sm text-content-secondary">
					No memories yet. The agent saves them as it learns durable facts about
					this project.
				</p>
			) : (
				<div className="mt-6 space-y-6">
					{memoryTypes.map((type) => {
						const typeMemories = memories.filter(
							(memory) => memory.type === type,
						);
						if (typeMemories.length === 0) return null;
						return (
							<div key={type} className="space-y-2">
								<h3 className="m-0 text-xs font-medium uppercase text-content-secondary">
									{type}
								</h3>
								{typeMemories.map((memory) => {
									const expanded = expandedMemoryIDs.has(memory.id);
									return (
										<div
											key={memory.id}
											className="rounded-md border border-border-default p-3"
										>
											<div className="flex items-start gap-2">
												<div className="min-w-0 flex-1">
													<div className="font-mono text-sm text-content-primary">
														{memory.name}
													</div>
													<p className="mb-0 mt-1 text-sm text-content-secondary">
														{memory.description}
													</p>
													<p className="mb-0 mt-1 text-xs text-content-secondary">
														updated {shortRelativeTime(memory.updated_at)} by{" "}
														{memory.created_by_username || "Unknown"}
													</p>
												</div>
												<Button
													variant="subtle"
													size="icon"
													aria-label={`Toggle ${memory.name}`}
													onClick={() =>
														setExpandedMemoryIDs((current) => {
															const next = new Set(current);
															if (next.has(memory.id)) next.delete(memory.id);
															else next.add(memory.id);
															return next;
														})
													}
												>
													<ChevronDownIcon
														className={expanded ? "rotate-180" : undefined}
													/>
												</Button>
												<DropdownMenu>
													<DropdownMenuTrigger asChild>
														<Button
															variant="subtle"
															size="icon"
															aria-label={`Open actions for ${memory.name}`}
														>
															<EllipsisVerticalIcon />
														</Button>
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end">
														<DropdownMenuItem
															onSelect={() => setEditingMemory(memory)}
														>
															Edit
														</DropdownMenuItem>
														<DropdownMenuItem
															className="text-content-destructive focus:text-content-destructive"
															onSelect={() => setDeletingMemory(memory)}
														>
															Delete
														</DropdownMenuItem>
													</DropdownMenuContent>
												</DropdownMenu>
											</div>
											{expanded && (
												<div className="mt-3 border-t border-border-default pt-3">
													<MemoizedMarkdown>{memory.body}</MemoizedMarkdown>
												</div>
											)}
										</div>
									);
								})}
							</div>
						);
					})}
				</div>
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
