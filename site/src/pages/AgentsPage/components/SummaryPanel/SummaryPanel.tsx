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
	additions: number;
	deletions: number;
}

interface SummaryPrompt {
	index: number;
	text: string;
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

const SectionDivider: FC = () => (
	<div className="border-0 border-t border-solid border-border-default" />
);

const MetadataRow: FC<{
	label: string;
	children: React.ReactNode;
	className?: string;
}> = ({ label, children, className }) => (
	<div className={cn("flex items-start gap-3 text-sm", className)}>
		<span className="w-28 shrink-0 text-content-secondary">{label}</span>
		<span className="min-w-0 flex-1 text-content-primary">{children}</span>
	</div>
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

const PRRow: FC<{ pr: SummaryPRDetail }> = ({ pr }) => (
	<div className="flex items-center gap-2 text-sm">
		<span className="min-w-0 flex-1 truncate text-content-primary">
			PR #{pr.number} &quot;{pr.title}&quot;
		</span>
		<GitPullRequestIcon className="size-4 shrink-0 text-content-secondary" />
		<DiffStatBadge additions={pr.additions} deletions={pr.deletions} />
	</div>
);

const FileRow: FC<{ file: SummaryFileChange }> = ({ file }) => (
	<div className="flex items-center gap-2 py-1.5 text-sm">
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
}) => {
	const [showAllPrompts, setShowAllPrompts] = useState(false);
	const [showAllActivities, setShowAllActivities] = useState(false);

	const hiddenPrompts = totalPrompts - prompts.length;
	const hiddenActivities = totalActivities - activities.length;

	return (
		<ScrollArea className="h-full" scrollBarClassName="w-1.5">
			<div className="flex flex-col gap-0">
				{/* Metadata section */}
				<div className="flex flex-col gap-2.5 px-4 py-4">
					<MetadataRow label="Created:">{metadata.createdAt}</MetadataRow>
					<MetadataRow label="Last updated:">
						{metadata.lastUpdatedAt}
					</MetadataRow>
					<MetadataRow label="Cost:">{metadata.costDisplay}</MetadataRow>
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
						<Badge variant="default" size="sm">
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

				<SectionDivider />

				{/* PR details */}
				{(prDetails.length > 0 || repo) && (
					<>
						<div className="flex flex-col gap-2 px-4 py-4">
							{prDetails.length > 0 && (
								<MetadataRow label="PR details:">
									<div className="flex flex-col gap-1.5">
										{prDetails.map((pr) => (
											<PRRow key={pr.number} pr={pr} />
										))}
									</div>
								</MetadataRow>
							)}
							{repo && <MetadataRow label="Repo:">{repo}</MetadataRow>}
						</div>
						<SectionDivider />
					</>
				)}

				{/* Prompt history */}
				<div className="flex flex-col gap-2 px-4 py-4">
					<h3 className="text-sm font-medium text-content-primary">
						Prompt history ({totalPrompts})
					</h3>
					<div className="flex flex-col gap-1.5">
						{(showAllPrompts ? prompts : prompts.slice(0, 3)).map((prompt) => (
							<div
								key={prompt.index}
								className="flex items-start gap-3 text-sm"
							>
								<span className="w-6 shrink-0 text-right tabular-nums text-content-secondary">
									{prompt.index}
								</span>
								<span className="text-content-primary">{prompt.text}</span>
							</div>
						))}
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

				<SectionDivider />

				{/* Activity */}
				<div className="flex flex-col gap-2 px-4 py-4">
					<h3 className="text-sm font-medium text-content-primary">
						Activity ({totalActivities})
					</h3>
					<div className="flex flex-col gap-2">
						{(showAllActivities ? activities : activities.slice(0, 3)).map(
							(activity) => (
								<div
									key={activity.text}
									className="flex items-start gap-2.5 text-sm"
								>
									<CheckIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
									<span className="text-content-primary">{activity.text}</span>
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

				<SectionDivider />

				{/* Files */}
				<div className="flex flex-col gap-2 px-4 py-4">
					<h3 className="text-sm font-medium text-content-primary">
						Files ({totalFiles})
					</h3>
					<div className="rounded-lg border border-solid border-border-default">
						<div className="flex flex-col divide-y divide-border-default px-3">
							{files.map((file) => (
								<FileRow key={file.path} file={file} />
							))}
						</div>
					</div>
				</div>

				<SectionDivider />

				{/* Related chats */}
				{relatedChats.length > 0 && (
					<div className="flex flex-col gap-2 px-4 py-4">
						<h3 className="text-sm font-medium text-content-primary">
							Related chats
						</h3>
						<div className="flex flex-col gap-1.5">
							{relatedChats.map((chat) => (
								<div
									key={chat.title}
									className="flex items-baseline gap-2 text-sm"
								>
									<span className="font-medium text-content-primary">
										{chat.title}
									</span>
									<span className="text-content-secondary">
										({chat.reason})
									</span>
								</div>
							))}
						</div>
					</div>
				)}
			</div>
		</ScrollArea>
	);
};
