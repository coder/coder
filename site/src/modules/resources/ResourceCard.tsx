import { Children, type FC, type JSX, useState } from "react";
import type { WorkspaceAgent, WorkspaceResource } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { CopyableValue } from "#/components/CopyableValue/CopyableValue";
import { MemoizedInlineMarkdown } from "#/components/Markdown/InlineMarkdown";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { ResourceAvatar } from "./ResourceAvatar";
import { SensitiveValue } from "./SensitiveValue";

interface ResourceCardProps {
	resource: WorkspaceResource;
	agentRow: (agent: WorkspaceAgent) => JSX.Element;
}

export const ResourceCard: FC<ResourceCardProps> = ({ resource, agentRow }) => {
	const [shouldDisplayAllMetadata, setShouldDisplayAllMetadata] =
		useState(false);
	const metadataToDisplay = resource.metadata ?? [];

	const visibleMetadata = shouldDisplayAllMetadata
		? metadataToDisplay
		: metadataToDisplay.slice(0, resource.daily_cost > 0 ? 3 : 4);

	const mLength =
		resource.daily_cost > 0
			? (resource.metadata?.length ?? 0) + 1
			: (resource.metadata?.length ?? 0);

	const gridWidth = mLength === 1 ? 1 : 4;

	return (
		<div
			key={resource.id}
			className="resource-card border border-solid border-border bg-surface-primary [&:not(:last-child)]:border-b-0 first:rounded-t-[8px] last:rounded-b-[8px]"
		>
			<div className="flex flex-row items-start gap-20 border-0 border-b border-solid border-border px-8 py-6 last:border-b-0 max-md:w-full max-md:overflow-auto">
				<div className="flex w-fit min-w-[220px] shrink-0 flex-row gap-2">
					<div>
						<ResourceAvatar resource={resource} />
					</div>
					<div className="font-normal text-sm leading-6">
						<div className="overflow-hidden text-ellipsis whitespace-nowrap text-xs text-content-secondary">
							{resource.type}
						</div>
						<div className="overflow-hidden text-ellipsis whitespace-nowrap">
							{resource.name}
						</div>
					</div>
				</div>

				<div
					className="grow-[2] grid gap-x-10 gap-y-6"
					style={{
						gridTemplateColumns: `repeat(${gridWidth}, minmax(0, 1fr))`,
					}}
				>
					{resource.daily_cost > 0 && (
						<div className="font-normal text-sm leading-6">
							<div className="overflow-hidden text-ellipsis whitespace-nowrap font-normal text-xs text-content-secondary">
								<b>Daily cost</b>
							</div>
							<div className="overflow-hidden text-ellipsis whitespace-nowrap">
								{resource.daily_cost}
							</div>
						</div>
					)}
					{visibleMetadata.map((meta) => {
						return (
							<div className="font-normal text-sm leading-6" key={meta.key}>
								<div className="overflow-hidden text-ellipsis whitespace-nowrap font-normal text-xs text-content-secondary">
									{meta.key}
								</div>
								<div className="overflow-hidden text-ellipsis whitespace-nowrap">
									{meta.sensitive ? (
										<SensitiveValue value={meta.value} />
									) : (
										<MemoizedInlineMarkdown
											components={{
												p: ({ children }) => {
													const childrens = Children.toArray(children);
													if (
														childrens.every(
															(child) => typeof child === "string",
														)
													) {
														return (
															<CopyableValue value={childrens.join("")}>
																{children}
															</CopyableValue>
														);
													}
													return <>{children}</>;
												},
											}}
										>
											{meta.value}
										</MemoizedInlineMarkdown>
									)}
								</div>
							</div>
						);
					})}
				</div>
				{mLength > 4 && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								onClick={() => {
									setShouldDisplayAllMetadata((value) => !value);
								}}
								size="icon-lg"
								variant="subtle"
							>
								<ChevronDownIcon open={shouldDisplayAllMetadata} />
							</Button>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							{shouldDisplayAllMetadata ? "Hide metadata" : "Show all metadata"}
						</TooltipContent>
					</Tooltip>
				)}
			</div>

			{resource.agents && resource.agents.length > 0 && (
				<div>{resource.agents.map(agentRow)}</div>
			)}
		</div>
	);
};
