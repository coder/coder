import { API } from "api/api";
import type { TemplateVersion } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Loader } from "components/Loader/Loader";
import {
	Sheet,
	SheetContent,
	SheetDescription,
	SheetHeader,
	SheetTitle,
} from "components/Sheet/Sheet";
import { FileTextIcon, FilterIcon, SearchIcon, XIcon } from "lucide-react";
import { useWatchVersionLogs } from "modules/templates/useWatchVersionLogs";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { type FC, useMemo, useState } from "react";
import { useQuery } from "react-query";

interface TemplateVersionLogsProps {
	version: TemplateVersion;
	open: boolean;
	onClose: () => void;
}

type LogLevel = "trace" | "debug" | "info" | "warn" | "error";

const LOG_LEVELS: LogLevel[] = ["trace", "debug", "info", "warn", "error"];

export const TemplateVersionLogs: FC<TemplateVersionLogsProps> = ({
	version,
	open,
	onClose,
}) => {
	const [searchQuery, setSearchQuery] = useState("");
	const [selectedLevels, setSelectedLevels] = useState<Set<LogLevel>>(
		new Set(LOG_LEVELS),
	);

	const jobStatus = version.job.status;
	const isRunning = jobStatus === "running" || jobStatus === "pending";

	// For completed builds, fetch logs once
	const { data: completedLogs } = useQuery({
		queryKey: ["templateVersion", version.id, "logs"],
		queryFn: () => API.getTemplateVersionLogs(version.id),
		enabled: !isRunning && open,
	});

	// For running builds, watch logs in real-time
	const runningLogs = useWatchVersionLogs(isRunning ? version : undefined);

	// Use the appropriate logs source
	const logs = isRunning ? runningLogs : completedLogs;

	// Filter logs based on search query and selected log levels
	const filteredLogs = useMemo(() => {
		if (!logs) return undefined;

		return logs.filter((log) => {
			// Filter by log level
			if (log.log_level && !selectedLevels.has(log.log_level)) {
				return false;
			}

			// Filter by search query
			if (searchQuery.trim()) {
				const query = searchQuery.toLowerCase();
				return (
					log.output?.toLowerCase().includes(query) ||
					log.stage?.toLowerCase().includes(query) ||
					log.log_source?.toLowerCase().includes(query)
				);
			}

			return true;
		});
	}, [logs, searchQuery, selectedLevels]);

	const toggleLogLevel = (level: LogLevel) => {
		setSelectedLevels((prev) => {
			const newSet = new Set(prev);
			if (newSet.has(level)) {
				newSet.delete(level);
			} else {
				newSet.add(level);
			}
			return newSet;
		});
	};

	const toggleAllLevels = () => {
		if (selectedLevels.size === LOG_LEVELS.length) {
			setSelectedLevels(new Set());
		} else {
			setSelectedLevels(new Set(LOG_LEVELS));
		}
	};

	const clearSearch = () => {
		setSearchQuery("");
	};

	const getJobStatusMessage = () => {
		switch (jobStatus) {
			case "pending":
				return "Build is pending...";
			case "running":
				return "Build is running...";
			case "succeeded":
				return "Build completed successfully";
			case "failed":
				return "Build failed";
			case "canceled":
			case "canceling":
				return "Build was canceled";
			default:
				return "Build logs";
		}
	};

	const hasLogs = filteredLogs && filteredLogs.length > 0;
	const allLogsFiltered = logs && logs.length > 0 && filteredLogs?.length === 0;

	return (
		<Sheet open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<SheetContent className="w-full sm:max-w-3xl overflow-hidden flex flex-col">
				<SheetHeader>
					<SheetTitle>{getJobStatusMessage()}</SheetTitle>
					<SheetDescription>
						Version {version.name} • Created by {version.created_by.username}
					</SheetDescription>
				</SheetHeader>

				{/* Search and Filter Controls */}
				<div className="flex flex-col gap-3 py-4 border-b border-border-primary">
					{/* Search Bar */}
					<div className="relative">
						<SearchIcon className="absolute left-3 top-1/2 -translate-y-1/2 size-icon-sm text-content-secondary" />
						<Input
							type="text"
							placeholder="Search logs..."
							value={searchQuery}
							onChange={(e) => setSearchQuery(e.target.value)}
							className="pl-10 pr-10"
						/>
						{searchQuery && (
							<button
								type="button"
								onClick={clearSearch}
								className="absolute right-3 top-1/2 -translate-y-1/2 text-content-secondary hover:text-content-primary"
								aria-label="Clear search"
							>
								<XIcon className="size-icon-sm" />
							</button>
						)}
					</div>

					{/* Log Level Filters */}
					<div className="flex flex-wrap items-center gap-2">
						<div className="flex items-center gap-2 text-sm text-content-secondary">
							<FilterIcon className="size-icon-sm" />
							<span>Filter by level:</span>
						</div>
						<Button
							variant="outline"
							size="sm"
							onClick={toggleAllLevels}
							className="h-7"
						>
							{selectedLevels.size === LOG_LEVELS.length
								? "Deselect All"
								: "Select All"}
						</Button>
						{LOG_LEVELS.map((level) => {
							const isSelected = selectedLevels.has(level);
							return (
								<Badge
									key={level}
									variant={isSelected ? "info" : "default"}
									className="cursor-pointer select-none capitalize"
									onClick={() => toggleLogLevel(level)}
									role="button"
									tabIndex={0}
									onKeyDown={(e) => {
										if (e.key === "Enter" || e.key === " ") {
											e.preventDefault();
											toggleLogLevel(level);
										}
									}}
								>
									{level}
								</Badge>
							);
						})}
					</div>

					{/* Results count */}
					{logs && (
						<div className="text-xs text-content-secondary">
							{allLogsFiltered ? (
								<span>No logs match the current filters</span>
							) : (
								<span>
									Showing {filteredLogs?.length ?? 0} of {logs.length} log
									{logs.length !== 1 ? "s" : ""}
								</span>
							)}
						</div>
					)}
				</div>

				{/* Logs Display */}
				<div className="flex-1 overflow-auto">
					{!logs ? (
						<div className="flex items-center justify-center h-32">
							<Loader />
						</div>
					) : allLogsFiltered ? (
						<div className="flex flex-col items-center justify-center h-32 text-content-secondary">
							<FileTextIcon className="size-icon-lg mb-2" />
							<p className="text-sm">
								No logs match your search or filter criteria
							</p>
							<Button
								variant="subtle"
								size="sm"
								onClick={() => {
									setSearchQuery("");
									setSelectedLevels(new Set(LOG_LEVELS));
								}}
								className="mt-2"
							>
								Clear all filters
							</Button>
						</div>
					) : hasLogs ? (
						<WorkspaceBuildLogs logs={filteredLogs} className="border-0" />
					) : (
						<div className="flex flex-col items-center justify-center h-32 text-content-secondary">
							<FileTextIcon className="size-icon-lg mb-2" />
							<p className="text-sm">
								{isRunning ? "Waiting for logs..." : "No logs available"}
							</p>
						</div>
					)}
				</div>
			</SheetContent>
		</Sheet>
	);
};
