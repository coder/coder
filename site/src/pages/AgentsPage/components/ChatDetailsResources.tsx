import {
	FileIcon,
	FolderIcon,
	PlugIcon,
	TriangleAlertIcon,
	WrenchIcon,
	ZapIcon,
} from "lucide-react";
import type { FC } from "react";
import type { ChatContextResource } from "#/api/typesGenerated";
import { formatKiB } from "#/utils/fileSize";
import {
	type ContextResourceItem,
	groupContextResources,
} from "../utils/chatDetails";

const ResourceMetadata: FC<{ resource: ChatContextResource }> = ({
	resource,
}) => (
	<>
		{Number.isFinite(resource.size_bytes) && resource.size_bytes > 0 && (
			<span className="text-xs text-content-secondary">
				{formatKiB(resource.size_bytes)}
			</span>
		)}
		{resource.error && (
			<p className="m-0 text-xs text-highlight-orange wrap-anywhere">
				{resource.error}
			</p>
		)}
	</>
);

export const ContextResourceGroups: FC<{
	items: readonly ContextResourceItem[];
	kind: "file" | "skill";
}> = ({ items, kind }) => {
	const Icon = kind === "file" ? FileIcon : ZapIcon;
	return (
		<div className="flex flex-col gap-4">
			{groupContextResources(items).map(({ dir, items }) => (
				<div key={dir} className="min-w-0">
					<div className="mb-2 flex items-start gap-2 text-xs text-content-secondary">
						<FolderIcon
							aria-hidden="true"
							className="mt-0.5 size-3.5 shrink-0"
						/>
						<span className="min-w-0 wrap-anywhere">
							{dir || "Current directory"}
						</span>
					</div>
					<ul className="m-0 ml-1.5 flex list-none flex-col gap-3 border-0 border-l border-solid border-border-default py-0 pl-4">
						{items.map(({ resource, name }) => (
							<li
								key={resource.source}
								className="flex min-w-0 items-start gap-2"
							>
								<Icon
									aria-hidden="true"
									className="mt-0.5 size-3.5 shrink-0 text-content-secondary"
								/>
								<div className="flex min-w-0 flex-col gap-1">
									<span className="wrap-anywhere">{name}</span>
									{kind === "skill" && (
										<span className="text-xs text-content-secondary wrap-anywhere">
											{resource.source || "Source unavailable"}
										</span>
									)}
									{resource.skill_description && (
										<p className="m-0 text-xs text-content-secondary wrap-anywhere">
											{resource.skill_description}
										</p>
									)}
									<ResourceMetadata resource={resource} />
								</div>
							</li>
						))}
					</ul>
				</div>
			))}
		</div>
	);
};

export const ContextResourceIssues: FC<{
	items: readonly ContextResourceItem[];
}> = ({ items }) =>
	items.length === 0 ? null : (
		<div className="flex flex-col gap-2">
			<p className="m-0 flex items-center gap-2 font-medium text-highlight-orange">
				<TriangleAlertIcon aria-hidden="true" className="size-3.5" />
				Resource issues
			</p>
			<ul className="m-0 flex list-none flex-col gap-3 p-0">
				{items.map(({ resource, name }) => (
					<li
						key={resource.source}
						className="flex flex-col gap-1 wrap-anywhere"
					>
						<span>
							{name}{" "}
							<span className="text-xs text-highlight-orange">
								(
								{resource.status === "ok"
									? "Missing resource name or path"
									: resource.status}
								)
							</span>
						</span>
						{resource.source && (
							<span className="text-xs text-content-secondary">
								{resource.source}
							</span>
						)}
						<ResourceMetadata resource={resource} />
					</li>
				))}
			</ul>
		</div>
	);

export const McpResourceList: FC<{
	configs: readonly ContextResourceItem[];
	servers: readonly ContextResourceItem[];
}> = ({ configs, servers }) => (
	<div className="flex flex-col gap-4">
		{configs.length > 0 && (
			<div className="flex flex-col gap-2">
				<h4 className="m-0 text-xs font-medium text-content-secondary">
					Configuration files
				</h4>
				<ul className="m-0 flex list-none flex-col gap-3 p-0">
					{configs.map(({ resource }) => (
						<li key={resource.source} className="flex items-start gap-2">
							<FileIcon
								aria-hidden="true"
								className="mt-0.5 size-3.5 shrink-0 text-content-secondary"
							/>
							<div className="flex min-w-0 flex-col gap-1">
								<span className="wrap-anywhere">{resource.source}</span>
								<ResourceMetadata resource={resource} />
							</div>
						</li>
					))}
				</ul>
			</div>
		)}
		<ul className="m-0 flex list-none flex-col gap-4 p-0">
			{servers.map(({ resource, name }) => (
				<li key={resource.source} className="flex min-w-0 flex-col gap-2">
					<div className="flex items-start gap-2">
						<PlugIcon
							aria-hidden="true"
							className="mt-0.5 size-3.5 shrink-0 text-content-secondary"
						/>
						<span className="min-w-0 wrap-anywhere">{name}</span>
					</div>
					<div className="ml-5 flex flex-col gap-2">
						<span
							className={
								resource.status === "ok"
									? "text-content-secondary"
									: "text-highlight-orange"
							}
						>
							{resource.status === "ok"
								? "Connected"
								: resource.status === "unreadable"
									? "Connection or tool discovery failed"
									: `Unavailable (${resource.status})`}
						</span>
						<ResourceMetadata resource={resource} />
						<span className="text-xs text-content-secondary">
							{(resource.tools ?? []).length}{" "}
							{(resource.tools ?? []).length === 1 ? "tool" : "tools"}
						</span>
						<ul className="m-0 flex list-none flex-col gap-3 p-0">
							{(resource.tools ?? []).map((tool) => (
								<li key={tool.name} className="flex items-start gap-2">
									<WrenchIcon
										aria-hidden="true"
										className="mt-0.5 size-3 shrink-0 text-content-secondary"
									/>
									<div className="min-w-0">
										<span className="wrap-anywhere">
											{tool.name || "Unnamed tool"}
										</span>
										{tool.description && (
											<p className="m-0 mt-1 text-xs text-content-secondary wrap-anywhere">
												{tool.description}
											</p>
										)}
									</div>
								</li>
							))}
						</ul>
					</div>
				</li>
			))}
		</ul>
	</div>
);
