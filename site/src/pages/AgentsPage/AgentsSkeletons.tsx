import { Skeleton } from "components/Skeleton/Skeleton";
import type { FC } from "react";

/**
 * Skeleton shown while the AgentsPage chunk is loading. Mimics the
 * sidebar + empty main area layout so the user sees structure
 * immediately instead of a fullscreen spinner.
 */
export const AgentsPageSkeleton: FC = () => (
	<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
		<div className="shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default md:h-full md:w-[320px] md:min-h-0 md:border-b-0">
			<div className="flex h-full w-full min-h-0 flex-col border-0 border-r border-solid">
				<div className="border-b border-border-default px-3 pb-3 pt-1.5 md:px-3.5">
					<Skeleton className="mb-2.5 h-6 w-6 rounded" />
					<div className="flex flex-col gap-2.5">
						<Skeleton className="h-9 w-full rounded-lg" />
						<Skeleton className="h-9 w-full rounded-lg" />
					</div>
				</div>
				<div className="flex flex-col gap-2 px-2 py-3">
					<Skeleton className="ml-2.5 h-3.5 w-16" />
					<div className="flex flex-col gap-0.5">
						{Array.from({ length: 6 }, (_, i) => (
							<div
								key={i}
								className="flex items-start gap-2 rounded-md px-2 py-1"
							>
								<Skeleton className="mt-0.5 h-5 w-5 shrink-0 rounded-md" />
								<div className="min-w-0 flex-1 space-y-1.5">
									<Skeleton
										className="h-3.5"
										style={{ width: `${55 + ((i * 17) % 35)}%` }}
									/>
									<Skeleton className="h-3 w-20" />
								</div>
							</div>
						))}
					</div>
				</div>
			</div>
		</div>
		<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary" />
	</div>
);

/**
 * Skeleton shown while the AgentDetail chunk is loading. Mimics a
 * chat conversation layout (user bubble + assistant response lines).
 */
export const AgentDetailSkeleton: FC = () => (
	<div className="mx-auto w-full max-w-3xl space-y-6 py-6">
		<div className="flex justify-end">
			<Skeleton className="h-10 w-2/3 rounded-xl" />
		</div>
		<div className="space-y-3">
			<Skeleton className="h-4 w-full" />
			<Skeleton className="h-4 w-5/6" />
			<Skeleton className="h-4 w-4/6" />
			<Skeleton className="h-4 w-full" />
			<Skeleton className="h-4 w-3/5" />
		</div>
	</div>
);
