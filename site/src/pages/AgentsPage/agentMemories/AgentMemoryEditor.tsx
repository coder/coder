import {
	ArrowLeftIcon,
	RotateCcwIcon,
	SaveIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useState } from "react";
import type { AgentMemory } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";

const MAX_MEMORY_BYTES = 64 * 1024;

type AgentMemoryEditorProps = {
	memory: AgentMemory;
	isSaving: boolean;
	saveError?: string;
	isConflict: boolean;
	onDirtyChange: (dirty: boolean) => void;
	onSave: (content: string) => void;
	onReloadLatest: () => Promise<AgentMemory | undefined>;
	onDelete: () => void;
	onBack: () => void;
};

const formatTimestamp = (value: string) => {
	const date = new Date(value);
	return Number.isFinite(date.getTime())
		? date.toLocaleString("en-US", { dateStyle: "medium", timeStyle: "short" })
		: "Unknown";
};

export const AgentMemoryEditor: FC<AgentMemoryEditorProps> = ({
	memory,
	isSaving,
	saveError,
	isConflict,
	onDirtyChange,
	onSave,
	onReloadLatest,
	onDelete,
	onBack,
}) => {
	const [savedContent, setSavedContent] = useState(memory.content);
	const [draft, setDraft] = useState(memory.content);
	const byteLength = new TextEncoder().encode(draft).byteLength;
	const isDirty = draft !== savedContent;
	const isOversized = byteLength > MAX_MEMORY_BYTES;

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-4 p-4 md:p-6">
			<div className="flex items-start gap-3">
				<Button
					className="md:hidden"
					variant="subtle"
					size="icon"
					aria-label="Back to memories"
					onClick={onBack}
				>
					<ArrowLeftIcon />
				</Button>
				<div className="min-w-0 flex-1">
					<h2 className="m-0 truncate font-mono text-base font-medium text-content-primary">
						{memory.path}
					</h2>
					<p className="m-0 mt-1 text-xs text-content-secondary">
						Created {formatTimestamp(memory.created_at)} · Updated{" "}
						{formatTimestamp(memory.updated_at)}
					</p>
				</div>
			</div>
			{isConflict && (
				<Alert
					severity="warning"
					actions={
						<Button
							size="sm"
							variant="outline"
							onClick={async () => {
								const latest = await onReloadLatest();
								if (latest) {
									setSavedContent(latest.content);
									setDraft(latest.content);
									onDirtyChange(false);
								}
							}}
						>
							Reload latest
						</Button>
					}
				>
					<AlertTitle>Memory changed elsewhere</AlertTitle>
					<AlertDescription>
						Your draft was preserved. Reload the latest version before editing
						again.
					</AlertDescription>
				</Alert>
			)}
			{saveError && !isConflict && (
				<Alert severity="error">
					<AlertDescription>{saveError}</AlertDescription>
				</Alert>
			)}
			<div className="flex min-h-0 flex-1 flex-col gap-2">
				<Textarea
					id={`memory-${memory.id}`}
					aria-label="Markdown"
					value={draft}
					onChange={(event) => {
						const next = event.currentTarget.value;
						setDraft(next);
						onDirtyChange(next !== savedContent);
					}}
					className="min-h-80 flex-1 resize-none font-mono"
					aria-describedby={
						isOversized ? `memory-${memory.id}-size` : undefined
					}
				/>
				<p
					id={`memory-${memory.id}-size`}
					className={
						isOversized
							? "m-0 text-xs text-content-destructive"
							: "m-0 text-xs text-content-secondary"
					}
				>
					{byteLength.toLocaleString("en-US")} /{" "}
					{MAX_MEMORY_BYTES.toLocaleString("en-US")} bytes
				</p>
			</div>
			<div className="flex flex-wrap justify-between gap-3 border-t border-border-default pt-4">
				<Button variant="destructive" onClick={onDelete}>
					<Trash2Icon />
					Delete
				</Button>
				<div className="flex gap-2">
					<Button
						variant="outline"
						disabled={!isDirty || isSaving}
						onClick={() => {
							setDraft(savedContent);
							onDirtyChange(false);
						}}
					>
						<RotateCcwIcon />
						Reset
					</Button>
					<Button
						disabled={!isDirty || isOversized || isSaving}
						onClick={() => onSave(draft)}
					>
						{isSaving ? <Spinner loading size="sm" /> : <SaveIcon />}Save
					</Button>
				</div>
			</div>
		</div>
	);
};
