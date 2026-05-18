import {
	ArrowDownIcon,
	ArrowUpIcon,
	CheckIcon,
	FileIcon,
	GitPullRequestIcon,
	LayersIcon,
	PenLineIcon,
	XIcon,
} from "lucide-react";
import type { FC } from "react";
import { useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { cn } from "#/utils/cn";
import { DiffStatBadge } from "../DiffViewer/DiffStats";

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

interface SummaryMetadata {
	createdAt: string;
	lastUpdatedAt: string;
	costDisplay: string;
	tokens: {
		input: number;
		cached: number;
		output: number;
	};
	model: string;
	tags: readonly string[];
}

interface SummaryPRDetail {
	number: number;
	title: string;
	url?: string;
	additions: number;
	deletions: number;
}

interface SummaryPrompt {
	/** 1-based prompt number shown in the UI. */
	index: number;
	text: string;
	/** Chat message ID this prompt corresponds to. */
	messageId?: number;
}

interface SummaryActivity {
	text: string;
}

interface SummaryFileChange {
	path: string;
	status: "New" | "Edited" | "Deleted";
	additions: number;
	deletions: number;
}

interface SummaryRelatedChat {
	title: string;
	reason: string;
	/** Agent chat ID to link to. */
	chatId?: string;
}

export interface SummaryPanelProps {
	metadata: SummaryMetadata;
	repo: string | undefined;
	prDetails: readonly SummaryPRDetail[];
	prompts: readonly SummaryPrompt[];
	totalPrompts: number;
	activities: readonly SummaryActivity[];
	totalActivities: number;
	files: readonly SummaryFileChange[];
	totalFiles: number;
	relatedChats: readonly SummaryRelatedChat[];
	onRemoveTag?: (tag: string) => void;
	onEditTags?: () => void;
	/** Called when the user clicks a prompt to scroll to that message. */
	onPromptClick?: (messageId: number) => void;
	/** Called when the user clicks a related chat link. */
	onRelatedChatClick?: (chatId: string) => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatCompactNumber(n: number): string {
	if (n >= 1_000_000) {
		const m = n / 1_000_000;
		return `${m % 1 === 0 ? m.toFixed(0) : m.toFixed(1)}M`;
	}
	return n.toLocaleString("en-US");
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/**
 * Section wrapper with left accent border, top divider, and overflow
 * containment so descendant `truncate` classes actually take effect.
 */
const Section: FC<{
	children: React.ReactNode;
	className?: string;
}> = ({ children, className }) => (
	<div
		className={cn(
			"min-w-0 overflow-hidden border-0 border-l-2 border-t border-solid border-border-default px-5 py-5",
			className,
		)}
	>
		{children}
	</div>
);

/**
 * Key-value row used in the metadata and PR details sections.
 * The value column is overflow-hidden so children can truncate.
 */
const MetadataRow: FC<{
	label: string;
	children: React.ReactNode;
}> = ({ label, children }) => (
	<div className="flex min-w-0 items-start gap-4 text-sm leading-6">
		<span className="w-[7.5rem] shrink-0 text-content-secondary">{label}</span>
		<div className="min-w-0 flex-1 overflow-hidden text-content-primary">
			{children}
		</div>
	</div>
);

/** Plain text value that truncates with an ellipsis. */
const MetadataValue: FC<{ children: React.ReactNode }> = ({ children }) => (
	<span className="block truncate">{children}</span>
);

const TokenBadge: FC<{
	icon: React.ReactNode;
	value: number;
}> = ({ icon, value }) => (
	<Badge variant="default" size="sm">
		{icon}
		{formatCompactNumber(value)}
	</Badge>
);

const TagBadge: FC<{
	label: string;
	onRemove?: () => void;
}> = ({ label, onRemove }) => (
	<Badge variant="default" size="sm" className="gap-1">
		{label}
		{onRemove && (
			<button
				type="button"
				onClick={onRemove}
				className="ml-0.5 flex items-center text-content-secondary hover:text-content-primary"
			>
				<XIcon className="size-3" />
			</button>
		)}
	</Badge>
);

const PRRow: FC<{ pr: SummaryPRDetail }> = ({ pr }) => {
	const label = `PR #${pr.number} "${pr.title}"`;
	return (
		<div className="flex min-w-0 items-center gap-2 text-sm leading-6">
			{pr.url ? (
				<a
					href={pr.url}
					target="_blank"
					rel="noopener noreferrer"
					className="min-w-0 flex-1 truncate text-content-primary hover:text-content-link hover:underline"
				>
					{label}
				</a>
			) : (
				<span className="min-w-0 flex-1 truncate text-content-primary">
					{label}
				</span>
			)}
			<GitPullRequestIcon className="size-4 shrink-0 text-content-secondary" />
			<DiffStatBadge additions={pr.additions} deletions={pr.deletions} />
		</div>
	);
};

const FileRow: FC<{ file: SummaryFileChange }> = ({ file }) => (
	<div className="flex min-w-0 items-center gap-2 py-2 text-sm">
		<FileIcon className="size-4 shrink-0 text-content-secondary" />
		<span className="min-w-0 flex-1 truncate font-mono text-[13px] text-content-primary">
			{file.path}
		</span>
		<span className="shrink-0 text-xs text-content-secondary">
			{file.status}
		</span>
		<span className="shrink-0 font-mono text-[13px]">
			{file.additions > 0 && (
				<span className="text-git-added-bright">+{file.additions}</span>
			)}
			{file.additions > 0 && file.deletions > 0 && " "}
			{file.deletions > 0 && (
				<span className="text-git-deleted-bright">-{file.deletions}</span>
			)}
		</span>
	</div>
);

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export const SummaryPanel: FC<SummaryPanelProps> = ({
	metadata,
	repo,
	prDetails,
	prompts,
	totalPrompts,
	activities,
	totalActivities,
	files,
	totalFiles,
	relatedChats,
	onRemoveTag,
	onEditTags,
	onPromptClick,
	onRelatedChatClick,
}) => {
	const [showAllPrompts, setShowAllPrompts] = useState(false);
	const [showAllActivities, setShowAllActivities] = useState(false);

	const hiddenPrompts = totalPrompts - prompts.length;
	const hiddenActivities = totalActivities - activities.length;

	return (
		<ScrollArea className="h-full" scrollBarClassName="w-1.5">
			<div className="flex flex-col">
				{/* ---- Metadata ---- */}
				<Section className="border-l-0 border-t-0">
					<div className="flex flex-col gap-3">
						<MetadataRow label="Created:">
							<MetadataValue>{metadata.createdAt}</MetadataValue>
						</MetadataRow>
						<MetadataRow label="Last updated:">
							<MetadataValue>{metadata.lastUpdatedAt}</MetadataValue>
						</MetadataRow>
						<MetadataRow label="Cost:">
							<MetadataValue>{metadata.costDisplay}</MetadataValue>
						</MetadataRow>
						<MetadataRow label="Tokens:">
							<div className="flex flex-wrap gap-1.5">
								<TokenBadge
									icon={<ArrowDownIcon className="size-3" />}
									value={metadata.tokens.input}
								/>
								<TokenBadge
									icon={<LayersIcon className="size-3" />}
									value={metadata.tokens.cached}
								/>
								<TokenBadge
									icon={<ArrowUpIcon className="size-3" />}
									value={metadata.tokens.output}
								/>
							</div>
						</MetadataRow>
						<MetadataRow label="Model:">
							<Badge
								variant="default"
								size="sm"
								className="max-w-full truncate"
							>
								{metadata.model}
							</Badge>
						</MetadataRow>
						<MetadataRow label="Tags:">
							<div className="flex flex-wrap items-center gap-1.5">
								{metadata.tags.map((tag) => (
									<TagBadge
										key={tag}
										label={tag}
										onRemove={onRemoveTag ? () => onRemoveTag(tag) : undefined}
									/>
								))}
								{onEditTags && (
									<button
										type="button"
										onClick={onEditTags}
										className="flex items-center text-content-secondary hover:text-content-primary"
									>
										<PenLineIcon className="size-4" />
									</button>
								)}
							</div>
						</MetadataRow>
					</div>
				</Section>

				{/* ---- PR details ---- */}
				{(prDetails.length > 0 || repo) && (
					<Section>
						<div className="flex min-w-0 flex-col gap-3">
							{prDetails.length > 0 && (
								<MetadataRow label="PR details:">
									<div className="flex min-w-0 flex-col gap-1.5">
										{prDetails.map((pr) => (
											<PRRow key={pr.number} pr={pr} />
										))}
									</div>
								</MetadataRow>
							)}
							{repo && (
								<MetadataRow label="Repo:">
									<MetadataValue>{repo}</MetadataValue>
								</MetadataRow>
							)}
						</div>
					</Section>
				)}

				{/* ---- Prompt history ---- */}
				<Section>
					<div className="flex min-w-0 flex-col gap-3">
						<h3 className="text-sm font-medium text-content-secondary">
							Prompt history ({totalPrompts})
						</h3>
						<div className="flex flex-col gap-2.5">
							{(showAllPrompts ? prompts : prompts.slice(0, 3)).map(
								(prompt) => (
									<div
										key={prompt.index}
										className="flex min-w-0 items-start gap-4 text-sm"
									>
										{prompt.messageId && onPromptClick ? (
											<button
												type="button"
												onClick={() => onPromptClick(prompt.messageId!)}
												className="w-7 shrink-0 text-right tabular-nums text-content-link hover:underline"
											>
												{prompt.index}
											</button>
										) : (
											<span className="w-7 shrink-0 text-right tabular-nums text-content-secondary">
												{prompt.index}
											</span>
										)}
										<span className="min-w-0 flex-1 truncate text-content-primary">
											{prompt.text}
										</span>
									</div>
								),
							)}
						</div>
						{!showAllPrompts && hiddenPrompts > 0 && (
							<button
								type="button"
								onClick={() => setShowAllPrompts(true)}
								className="self-start text-sm text-content-link hover:underline"
							>
								+{hiddenPrompts} earlier
							</button>
						)}
					</div>
				</Section>

				{/* ---- Activity ---- */}
				<Section>
					<div className="flex min-w-0 flex-col gap-3">
						<h3 className="text-sm font-medium text-content-secondary">
							Activity ({totalActivities})
						</h3>
						<div className="flex flex-col gap-2.5">
							{(showAllActivities ? activities : activities.slice(0, 3)).map(
								(activity) => (
									<div
										key={activity.text}
										className="flex min-w-0 items-start gap-3 text-sm"
									>
										<CheckIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
										<span className="min-w-0 flex-1 truncate text-content-primary">
											{activity.text}
										</span>
									</div>
								),
							)}
						</div>
						{!showAllActivities && hiddenActivities > 0 && (
							<button
								type="button"
								onClick={() => setShowAllActivities(true)}
								className="self-start text-sm text-content-link hover:underline"
							>
								+{hiddenActivities} more
							</button>
						)}
					</div>
				</Section>

				{/* ---- Files ---- */}
				<Section>
					<div className="flex min-w-0 flex-col gap-3">
						<h3 className="text-sm font-medium text-content-secondary">
							Files ({totalFiles})
						</h3>
						<div className="min-w-0 overflow-hidden rounded-lg border border-solid border-border-default">
							<div className="flex flex-col divide-y divide-border-default px-3">
								{files.map((file) => (
									<FileRow key={`${file.path}-${file.status}`} file={file} />
								))}
							</div>
						</div>
					</div>
				</Section>

				{/* ---- Related chats ---- */}
				{relatedChats.length > 0 && (
					<Section>
						<div className="flex min-w-0 flex-col gap-3">
							<h3 className="text-sm font-medium text-content-secondary">
								Related chats
							</h3>
							<div className="flex flex-col gap-2">
								{relatedChats.map((chat) => (
									<div
										key={chat.title}
										className="flex min-w-0 items-baseline gap-2 text-sm"
									>
										{chat.chatId && onRelatedChatClick ? (
											<button
												type="button"
												onClick={() => onRelatedChatClick(chat.chatId!)}
												className="min-w-0 shrink truncate text-left font-medium text-content-primary hover:text-content-link hover:underline"
											>
												{chat.title}
											</button>
										) : (
											<span className="min-w-0 shrink truncate font-medium text-content-primary">
												{chat.title}
											</span>
										)}
										<span className="shrink-0 text-content-secondary">
											({chat.reason})
										</span>
									</div>
								))}
							</div>
						</div>
					</Section>
				)}
			</div>
		</ScrollArea>
	);
};
