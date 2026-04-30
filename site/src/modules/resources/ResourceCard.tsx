import type { Interpolation, Theme } from "@emotion/react";
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

const styles = {
	resourceCard: (theme) => ({
		border: `1px solid ${theme.palette.divider}`,
		background: theme.palette.background.default,

		"&:not(:last-child)": {
			borderBottom: 0,
		},

		"&:first-of-type": {
			borderTopLeftRadius: 8,
			borderTopRightRadius: 8,
		},

		"&:last-child": {
			borderBottomLeftRadius: 8,
			borderBottomRightRadius: 8,
		},
	}),

	resourceCardProfile: {
		flexShrink: 0,
		width: "fit-content",
		minWidth: 220,
	},

	resourceCardHeader: (theme) => ({
		padding: "24px 32px",
		borderBottom: `1px solid ${theme.palette.divider}`,

		"&:last-child": {
			borderBottom: 0,
		},

		[theme.breakpoints.down("md")]: {
			width: "100%",
			overflow: "scroll",
		},
	}),

	metadata: () => ({
		lineHeight: "1.5",
		fontSize: 14,
	}),

	metadataLabel: (theme) => ({
		fontSize: 12,
		color: theme.palette.text.secondary,
		textOverflow: "ellipsis",
		overflow: "hidden",
		whiteSpace: "nowrap",
	}),

	metadataValue: () => ({
		textOverflow: "ellipsis",
		overflow: "hidden",
		whiteSpace: "nowrap",
	}),
} satisfies Record<string, Interpolation<Theme>>;

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
		<div key={resource.id} css={styles.resourceCard} className="resource-card">
			<div
				className="flex flex-row items-start gap-20"
				css={styles.resourceCardHeader}
			>
				<div className="flex flex-row gap-2" css={styles.resourceCardProfile}>
					<div>
						<ResourceAvatar resource={resource} />
					</div>
					<div css={styles.metadata}>
						<div css={styles.metadataLabel}>{resource.type}</div>
						<div css={styles.metadataValue}>{resource.name}</div>
					</div>
				</div>

				<div
					className="grow-[2] grid gap-x-10 gap-y-6"
					style={{
						gridTemplateColumns: `repeat(${gridWidth}, minmax(0, 1fr))`,
					}}
				>
					{resource.daily_cost > 0 && (
						<div css={styles.metadata}>
							<div css={styles.metadataLabel}>
								<b>Daily cost</b>
							</div>
							<div css={styles.metadataValue}>{resource.daily_cost}</div>
						</div>
					)}
					{visibleMetadata.map((meta) => {
						return (
							<div css={styles.metadata} key={meta.key}>
								<div css={styles.metadataLabel}>{meta.key}</div>
								<div css={styles.metadataValue}>
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
