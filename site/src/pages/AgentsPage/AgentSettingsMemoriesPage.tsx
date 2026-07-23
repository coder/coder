import { isAxiosError } from "axios";
import { type FC, useEffect, useState } from "react";
import {
	type InfiniteData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import {
	agentMemoriesKey,
	agentMemory,
	agentMemoryDefault,
	deleteAgentMemory,
	updateAgentMemory,
} from "#/api/queries/agentMemories";
import type {
	AgentMemoryChildrenResponse,
	AgentMemoryEntry,
} from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { ConfirmDeleteDialog } from "#/components/Dialogs/ConfirmDeleteDialog/ConfirmDeleteDialog";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { AgentMemoryEditor } from "./agentMemories/AgentMemoryEditor";
import { AgentMemoryTree } from "./agentMemories/AgentMemoryTree";

const ancestorDirectories = (path: string) => {
	const segments = path.split("/").filter(Boolean).slice(0, -1);
	const ancestors: string[] = [];
	let current = "";
	for (const segment of segments) {
		current += `/${segment}`;
		ancestors.push(current);
	}
	return ancestors;
};

const errorMessage = (error: unknown, action: string) => {
	const status = isAxiosError(error) ? error.response?.status : undefined;
	if (status === 403)
		return "You do not have permission to manage agent memories.";
	if (status === 404) return "That memory no longer exists.";
	if (status === 400) return `The server rejected the memory ${action}.`;
	return `Could not ${action} the memory.`;
};

const AgentSettingsMemoriesPage: FC = () => {
	const queryClient = useQueryClient();
	const [selectedID, setSelectedID] = useState<string>();
	const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
	const [isDirty, setIsDirty] = useState(false);
	const [pendingSelection, setPendingSelection] = useState<AgentMemoryEntry>();
	const [isDeleteOpen, setIsDeleteOpen] = useState(false);
	const [mobileEditor, setMobileEditor] = useState(false);
	const [editorRevision, setEditorRevision] = useState(0);

	const defaultQuery = useQuery({
		...agentMemoryDefault(),
		retry: (_count, error) =>
			!(isAxiosError(error) && error.response?.status === 404),
	});
	const effectiveID = selectedID ?? defaultQuery.data?.id ?? "";
	const memoryQuery = useQuery({
		...agentMemory(effectiveID),
		enabled: Boolean(effectiveID),
		initialData:
			defaultQuery.data?.id === effectiveID ? defaultQuery.data : undefined,
	});
	const memory = memoryQuery.data;

	useEffect(() => {
		if (!memory) return;
		setExpanded(
			(current) => new Set([...current, ...ancestorDirectories(memory.path)]),
		);
	}, [memory]);

	const updateOptions = updateAgentMemory(queryClient);
	const updateMutation = useMutation({
		...updateOptions,
		onSuccess: async (updated) => {
			await updateOptions.onSuccess?.(updated);
			setIsDirty(false);
			setEditorRevision((value) => value + 1);
		},
	});
	const deleteOptions = deleteAgentMemory(queryClient);
	const deleteMutation = useMutation({
		...deleteOptions,
		onSuccess: async (data, deleted) => {
			const visibleMemories = queryClient
				.getQueriesData<InfiniteData<AgentMemoryChildrenResponse>>({
					queryKey: [...agentMemoriesKey(), "children"],
				})
				.flatMap(
					([, cached]) => cached?.pages.flatMap((page) => page.entries) ?? [],
				)
				.filter(
					(entry) =>
						entry.kind === "memory" && entry.id && entry.id !== deleted.id,
				)
				.toSorted((a, b) => a.path.localeCompare(b.path, "en-US"));
			const nextMemory =
				visibleMemories.find((entry) => entry.path > deleted.path) ??
				visibleMemories.at(-1);
			await deleteOptions.onSuccess?.(data, deleted);
			setIsDeleteOpen(false);
			setIsDirty(false);
			setSelectedID(nextMemory?.id);
			setMobileEditor(Boolean(nextMemory));
			await queryClient.invalidateQueries({ queryKey: agentMemoriesKey() });
		},
	});
	const unsavedNavigation = useUnsavedChangesPrompt(isDirty);

	const selectMemory = (entry: AgentMemoryEntry) => {
		if (!entry.id || entry.id === effectiveID) return;
		if (isDirty) {
			setPendingSelection(entry);
			return;
		}
		setSelectedID(entry.id);
		setMobileEditor(true);
		updateMutation.reset();
	};

	const defaultMissing =
		isAxiosError(defaultQuery.error) &&
		defaultQuery.error.response?.status === 404;

	return (
		<div className="flex min-h-[32rem] flex-col gap-4">
			<div>
				<h1 className="m-0 text-2xl font-semibold text-content-primary">
					Memories
				</h1>
				<p className="m-0 mt-1 text-sm text-content-secondary">
					Review and edit Markdown memories created by your agents.
				</p>
			</div>
			{defaultQuery.isLoading ? (
				<Loader />
			) : defaultQuery.error && !defaultMissing ? (
				<div className="flex flex-col items-start gap-3">
					<Alert severity="error">
						<AlertDescription>
							{errorMessage(defaultQuery.error, "load")}
						</AlertDescription>
					</Alert>
					<Button
						variant="outline"
						onClick={() => void defaultQuery.refetch()}
						disabled={defaultQuery.isRefetching}
					>
						{defaultQuery.isRefetching && <Spinner loading size="sm" />}Retry
					</Button>
				</div>
			) : defaultMissing ? (
				<EmptyState
					message="No memories yet"
					description="Memories will appear here after an agent saves information for you."
				/>
			) : (
				<div className="flex min-h-[30rem] flex-1 overflow-hidden rounded-lg border border-border-default bg-surface-primary">
					<aside
						className={`${mobileEditor ? "hidden" : "flex"} w-full min-w-0 flex-col border-r border-border-default md:flex md:w-72 md:shrink-0`}
					>
						<div className="border-b border-border-default px-4 py-3 text-sm font-medium text-content-primary">
							Memory files
						</div>
						<div className="min-h-0 flex-1 overflow-auto p-2">
							<AgentMemoryTree
								selectedPath={memory?.path}
								expanded={expanded}
								onToggle={(path) =>
									setExpanded((current) => {
										const next = new Set(current);
										if (next.has(path)) next.delete(path);
										else next.add(path);
										return next;
									})
								}
								onSelect={selectMemory}
							/>
						</div>
					</aside>
					<main
						className={`${mobileEditor ? "flex" : "hidden"} min-w-0 flex-1 md:flex`}
					>
						{memoryQuery.isLoading ? (
							<div className="m-auto">
								<Loader />
							</div>
						) : memoryQuery.error || !memory ? (
							<div className="flex flex-1 flex-col items-start gap-3 p-6">
								<Alert severity="error">
									<AlertDescription>
										{errorMessage(memoryQuery.error, "load")}
									</AlertDescription>
								</Alert>
								<Button
									variant="outline"
									onClick={() => void memoryQuery.refetch()}
								>
									Retry
								</Button>
							</div>
						) : (
							<AgentMemoryEditor
								key={`${memory.id}-${editorRevision}`}
								memory={memory}
								isSaving={updateMutation.isPending}
								isConflict={
									isAxiosError(updateMutation.error) &&
									updateMutation.error.response?.status === 409
								}
								saveError={
									updateMutation.error
										? errorMessage(updateMutation.error, "save")
										: undefined
								}
								onDirtyChange={setIsDirty}
								onSave={(content) =>
									updateMutation.mutate({
										memoryID: memory.id,
										request: {
											content,
											expected_updated_at: memory.updated_at,
										},
									})
								}
								onReloadLatest={async () => {
									const result = await memoryQuery.refetch();
									updateMutation.reset();
									return result.data;
								}}
								onDelete={() => setIsDeleteOpen(true)}
								onBack={() => setMobileEditor(false)}
							/>
						)}
					</main>
				</div>
			)}

			<ConfirmDialog
				open={Boolean(pendingSelection)}
				title="Discard unsaved changes?"
				description="Your edits to this memory will be lost."
				confirmText="Discard changes"
				hideCancel={false}
				onClose={() => setPendingSelection(undefined)}
				onConfirm={() => {
					if (pendingSelection?.id) {
						setIsDirty(false);
						setSelectedID(pendingSelection.id);
						setMobileEditor(true);
					}
					setPendingSelection(undefined);
				}}
			/>
			<ConfirmDialog
				open={unsavedNavigation.isOpen}
				title="Discard unsaved changes?"
				description="Your edits to this memory will be lost."
				confirmText="Discard changes"
				hideCancel={false}
				onClose={unsavedNavigation.onCancel}
				onConfirm={unsavedNavigation.onConfirm}
			/>
			{memory && (
				<ConfirmDeleteDialog
					open={isDeleteOpen}
					onOpenChange={setIsDeleteOpen}
					entity="memory"
					description={
						<>
							Delete <strong>{memory.path}</strong>? This cannot be undone.
						</>
					}
					onConfirm={() => deleteMutation.mutate(memory)}
					isPending={deleteMutation.isPending}
				>
					{deleteMutation.error && (
						<Alert severity="error">
							<AlertDescription>
								{errorMessage(deleteMutation.error, "delete")}
							</AlertDescription>
						</Alert>
					)}
				</ConfirmDeleteDialog>
			)}
		</div>
	);
};

export default AgentSettingsMemoriesPage;
