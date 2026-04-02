import type { FC } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";

export const SessionTimelineSkeleton: FC = () => {
	return (
		<div className="relative">
			<div className="grid grid-cols-[16px_1rem_1px_1fr_auto_16px]">
				{/* row 1: session start */}
				<div className="row-start-1 col-start-2 relative h-10 py-1">
					<StatusIndicatorDot
						variant="inactive"
						className="absolute right-0 translate-x-1/2 translate-y-1/2"
					/>
				</div>
				<div className="row-start-1 col-start-4 col-span-2 flex items-center h-10">
					<span className="text-content-secondary font-normal ml-4 py-1 text-sm">
						Session started
					</span>
				</div>

				{/* row 2: vertical line */}
				<div className="row-start-2 col-start-3 border-0 border-l border-solid" />

				{/* row 3: space above timeline border */}
				<div className="row-start-3 col-start-3 border-0 border-l border-t border-solid h-6" />

				{/* row 4: top border */}
				<div className="row-start-4 col-start-1 border-0 border-l border-t border-dashed border-surface-green rounded-tl-lg size-4" />
				<div className="row-start-4 col-start-2 border-0 border-t border-dashed border-surface-green" />
				<div className="row-start-4 col-start-3 border-0 border-l border-solid" />
				<div className="row-start-4 col-start-4 border-0 border-t border-dashed border-surface-green" />
				<div className="row-start-4 col-start-6 border-0 border-r border-t border-dashed border-surface-green rounded-tr-lg size-4" />

				{/* row 5: skeleton thread cards */}
				<div className="row-start-5 col-start-1 border-0 border-l border-dashed border-surface-green" />
				<div className="row-start-5 col-start-2 col-span-4 flex flex-col gap-4 py-2">
					{[0, 1, 2].map((i) => (
						<div
							key={i}
							className="border border-solid rounded-md flex flex-col lg:flex-row gap-6 p-2"
						>
							{/* avatar + username */}
							<div className="flex flex-row items-center gap-2">
								<Skeleton className="size-6 rounded-full flex-shrink-0" />
								<Skeleton className="h-4 w-20" />
							</div>
							{/* prompt */}
							<div className="flex-grow flex flex-col gap-2">
								<Skeleton className="h-3 w-12" />
								<Skeleton className="h-16 w-full" />
							</div>
							{/* right-column details */}
							<div className="flex flex-col gap-2 lg:w-64 flex-shrink-0">
								<Skeleton className="h-3 w-full" />
								<Skeleton className="h-3 w-4/5" />
								<Skeleton className="h-3 w-3/5" />
							</div>
						</div>
					))}
				</div>
				<div className="row-start-5 col-start-6 border-0 border-r border-dashed border-surface-green" />

				{/* row 6: bottom border */}
				<div className="row-start-6 col-start-1 border-0 border-l border-b border-dashed border-surface-green rounded-bl-lg size-4" />
				<div className="row-start-6 col-start-2 border-0 border-b border-dashed border-surface-green" />
				<div className="row-start-6 col-start-3 border-0 border-l border-solid" />
				<div className="row-start-6 col-start-4 col-span-2 border-0 border-b border-dashed border-surface-green" />
				<div className="row-start-6 col-start-6 border-0 border-r border-b border-dashed border-surface-green rounded-br-lg size-4" />

				{/* row 7: space below timeline border */}
				<div className="row-start-7 col-start-3 border-0 border-l border-t border-solid h-4" />

				{/* row 8: session end placeholder */}
				<div className="row-start-8 col-start-2 relative">
					<Skeleton className="size-2 rounded-full absolute right-0 translate-x-1/2 translate-y-1/2" />
				</div>
				<div className="row-start-8 col-start-4 flex items-center">
					<Skeleton className="h-4 w-32 ml-4" />
				</div>
			</div>
		</div>
	);
};
