import type { WorkspaceResource } from "api/typesGenerated";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { SensitiveValue } from "modules/resources/SensitiveValue";
import { Children, type FC, type HTMLAttributes } from "react";
import { cn } from "utils/cn";

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
		<header
			{...headerProps}
			className={cn(
				"p-6 flex flex-wrap gap-12 gap-y-6 mb-6 text-sm",
				"bg-gradient-to-b from-surface-primary via-surface-primary via-25% to-transparent",
				headerProps.className,
			)}
		>
			{metadata.map((meta) => {
				return (
					<div className="leading-normal" key={meta.key}>
						<div className="text-ellipsis overflow-hidden whitespace-nowrap">
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
						<div className="text-[13px] text-content-secondary text-ellipsis overflow-hidden whitespace-nowrap">
							{meta.key}
						</div>
					</div>
				);
			})}
		</header>
	);
};
