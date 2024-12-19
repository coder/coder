import type { Interpolation, Theme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import type { WorkspaceAgent, WorkspaceResource } from "api/typesGenerated";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { Stack } from "components/Stack/Stack";
import { Children, type FC, useState } from "react";
import { ResourceAvatar } from "./ResourceAvatar";
import { SensitiveValue } from "./SensitiveValue";

const styles = {
	resourceCard: (theme) => ({
		border: `1px solid ${theme.palette.divider}`,
		background: theme.palette.background.default,

		"&:not(:last-child)": {
			borderBottom: 0,
		},

		"&:first-child": {
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

export interface ResourceCardProps {
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
			<Stack
				direction="row"
				alignItems="flex-start"
				css={styles.resourceCardHeader}
				spacing={10}
			>
				<Stack direction="row" spacing={1} css={styles.resourceCardProfile}>
					<div>
						<ResourceAvatar resource={resource} />
					</div>
					<div css={styles.metadata}>
						<div css={styles.metadataLabel}>{resource.type}</div>
						<div css={styles.metadataValue}>{resource.name}</div>
					</div>
				</Stack>

				<div
					css={{
						flexGrow: 2,
						display: "grid",
						gridTemplateColumns: `repeat(${gridWidth}, minmax(0, 1fr))`,
						gap: 40,
						rowGap: 24,
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
					<Tooltip
						title={
							shouldDisplayAllMetadata ? "Hide metadata" : "Show all metadata"
						}
					>
						<IconButton
							onClick={() => {
								setShouldDisplayAllMetadata((value) => !value);
							}}
							size="large"
						>
							<DropdownArrow margin={false} close={shouldDisplayAllMetadata} />
						</IconButton>
					</Tooltip>
				)}
			</Stack>

			{resource.agents && resource.agents.length > 0 && (
				<div>{resource.agents.map(agentRow)}</div>
			)}
		</div>
	);
};
