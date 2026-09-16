import { type FC, useState } from "react";
import type { WorkspaceResource } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Sidebar,
	SidebarCaption,
	SidebarItem,
} from "#/components/FullPageLayout/Sidebar";
import { Skeleton } from "#/components/Skeleton/Skeleton";
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
	const [showHiddenResources, setShowHiddenResources] = useState(false);
	const hasHiddenResources = resources.some((r) => r.hide);
	const displayedResources = resources.filter(
		(r) => !r.hide || showHiddenResources || isSelected(r),
	);

	return (
		<Sidebar>
			<SidebarCaption>Resources</SidebarCaption>
			{failed && (
				<p className="m-0 py-4 text-sm font-normal text-content-secondary leading-normal">
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
			{displayedResources.map((r) => (
				<SidebarItem
					onClick={() => onChange(r)}
					isActive={isSelected(r)}
					key={r.id}
					className="leading-normal flex items-center gap-3"
				>
					<div className="flex items-center justify-center leading-none size-4 p-0.5">
						<ExternalImage
							className="w-full h-full object-contain"
							src={getResourceIconPath(r.type)}
							alt=""
						/>
					</div>
					<div className="flex flex-col font-medium">
						<span>{r.name}</span>
						<span className="text-sm text-content-secondary">{r.type}</span>
					</div>
				</SidebarItem>
			))}
			{hasHiddenResources && (
				<div className="flex items-center justify-center mt-4 px-4">
					<Button
						variant="outline"
						size="sm"
						className="rounded-full w-full"
						onClick={() => setShowHiddenResources((v) => !v)}
					>
						{showHiddenResources ? "Hide" : "Show hidden"} resources
						<ChevronDownIcon open={showHiddenResources} className="ml-2" />
					</Button>
				</div>
			)}
		</Sidebar>
	);
};

const ResourceSidebarItemSkeleton: FC = () => {
	return (
		<div className="leading-normal flex items-center gap-3 pointer-events-none">
			<Skeleton variant="circular" width={16} height={16} />
			<div>
				<Skeleton variant="text" width={94} height={16} />
				<Skeleton variant="text" width={60} height={14} className="mt-0.5" />
			</div>
		</div>
	);
};
