import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import type { FC } from "react";
import type { WorkspaceResource } from "#/api/typesGenerated";
import {
	Sidebar,
	SidebarCaption,
	SidebarItem,
} from "#/components/FullPageLayout/Sidebar";
import { getResourceIconPath } from "#/utils/workspace";

type ResourcesSidebarProps = {
	failed: boolean;
	resources: WorkspaceResource[];
	onChange: (resource: WorkspaceResource) => void;
	isSelected: (resource: WorkspaceResource) => boolean;
};

export const ResourcesSidebar: FC<ResourcesSidebarProps> = ({
	failed,
	onChange,
	isSelected,
	resources,
}) => {
	const theme = useTheme();

	return (
		<Sidebar>
			<SidebarCaption>Resources</SidebarCaption>
			{failed && (
				<p
					css={{
						margin: 0,
						padding: "0 16px",
						fontSize: 13,
						color: theme.palette.text.secondary,
						lineHeight: "1.5",
					}}
				>
					Your workspace build failed, so the necessary resources couldn&apos;t
					be created.
				</p>
			)}
			{resources.length === 0 &&
				!failed &&
				Array.from({ length: 8 }, (_, i) => (
					<SidebarItem key={i}>
						<ResourceSidebarItemSkeleton />
					</SidebarItem>
				))}
			{resources.map((r) => (
				<SidebarItem
					onClick={() => onChange(r)}
					isActive={isSelected(r)}
					key={r.id}
					css={styles.root}
				>
					<div className="flex items-center justify-center leading-none w-4 h-4 p-0.5">
						<img
							className="w-full h-full object-contain"
							src={getResourceIconPath(r.type)}
							alt=""
						/>
					</div>
					<div className="flex flex-col font-medium">
						<span>{r.name}</span>
						<span css={{ fontSize: 12, color: theme.palette.text.secondary }}>
							{r.type}
						</span>
					</div>
				</SidebarItem>
			))}
		</Sidebar>
	);
};

const ResourceSidebarItemSkeleton: FC = () => {
	return (
		<div css={[styles.root, { pointerEvents: "none" }]}>
			<Skeleton variant="circular" width={16} height={16} />
			<div>
				<Skeleton variant="text" width={94} height={16} />
				<Skeleton variant="text" width={60} height={14} className="mt-0.5" />
			</div>
		</div>
	);
};

const styles = {
	root: {
		lineHeight: "1.5",
		display: "flex",
		alignItems: "center",
		gap: 12,
	},
} satisfies Record<string, Interpolation<Theme>>;
