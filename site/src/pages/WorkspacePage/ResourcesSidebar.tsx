import Skeleton from "@mui/material/Skeleton";
import type { WorkspaceResource } from "api/typesGenerated";
import {
	Sidebar,
	SidebarCaption,
	SidebarItem,
} from "components/FullPageLayout/Sidebar";
import type { FC } from "react";
import { cn } from "utils/cn";
import { getResourceIconPath } from "utils/workspace";

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
	return (
		<Sidebar>
			<SidebarCaption>Resources</SidebarCaption>
			{failed && (
				<p className="m-0 px-4 text-[13px] text-content-secondary leading-normal">
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
					className={classNames.root}
				>
					<div className="flex items-center justify-center leading-[0] w-4 h-4 p-0.5">
						<img
							className="w-full h-full object-contain"
							src={getResourceIconPath(r.type)}
							alt=""
						/>
					</div>
					<div className="flex flex-col font-medium">
						<span>{r.name}</span>
						<span className="text-xs leading-[18px] text-content-secondary">
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
		<div className={cn([classNames.root, "pointer-events-none"])}>
			<Skeleton variant="circular" width={16} height={16} />
			<div>
				<Skeleton variant="text" width={94} height={16} />
				<Skeleton variant="text" width={60} height={14} className="mt-0.5" />
			</div>
		</div>
	);
};

const classNames = {
	root: "leading-normal flex items-center gap-3",
};
