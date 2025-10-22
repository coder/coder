import type { Interpolation, Theme } from "@emotion/react";
import type { WorkspaceResource } from "api/typesGenerated";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { SensitiveValue } from "modules/resources/SensitiveValue";
import { Children, type FC, type HTMLAttributes } from "react";

type ResourceMetadataProps = Omit<HTMLAttributes<HTMLElement>, "resource"> & {
	resource: WorkspaceResource;
};

export const ResourceMetadata: FC<ResourceMetadataProps> = ({
	resource,
	...headerProps
}) => {
	const metadata = resource.metadata ? [...resource.metadata] : [];

	if (resource.daily_cost > 0) {
		metadata.push({
			key: "Daily cost",
			value: resource.daily_cost.toString(),
			sensitive: false,
		});
	}

	if (metadata.length === 0) {
		return null;
	}

	return (
		<header css={styles.root} {...headerProps}>
			{metadata.map((meta) => {
				return (
					<div css={styles.item} key={meta.key}>
						<div css={styles.value}>
							{meta.sensitive ? (
								<SensitiveValue value={meta.value} />
							) : (
								<MemoizedInlineMarkdown
									components={{
										p: ({ children }) => {
											const childrenArray = Children.toArray(children);
											if (
												childrenArray.every(
													(child) => typeof child === "string",
												)
											) {
												return (
													<CopyableValue value={childrenArray.join("")}>
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
						<div css={styles.label}>{meta.key}</div>
					</div>
				);
			})}
		</header>
	);
};

const styles = {
	root: (theme) => ({
		padding: 24,
		display: "flex",
		flexWrap: "wrap",
		gap: 48,
		rowGap: 24,
		marginBottom: 24,
		fontSize: 14,
		background: `linear-gradient(180deg, ${theme.palette.background.default} 25%, rgba(0, 0, 0, 0) 100%)`,
	}),

	item: {
		lineHeight: "1.5",
	},

	label: (theme) => ({
		fontSize: 13,
		color: theme.palette.text.secondary,
		textOverflow: "ellipsis",
		overflow: "hidden",
		whiteSpace: "nowrap",
	}),

	value: {
		textOverflow: "ellipsis",
		overflow: "hidden",
		whiteSpace: "nowrap",
	},
} satisfies Record<string, Interpolation<Theme>>;
