import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "components/Breadcrumb/Breadcrumb";
import { Button } from "components/Button/Button";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Skeleton } from "components/Skeleton/Skeleton";
import type { FC } from "react";
import { Link } from "react-router";
import { DiagnosticSummaryBar } from "./DiagnosticSummaryBar";
import { PatternBanner } from "./PatternBanner";
import { SessionList } from "./SessionList";
import type { UserDiagnosticResponse } from "./types";
import { UserSelector } from "./UserSelector";

interface OperatorDiagnosticPageViewProps {
	data: UserDiagnosticResponse | undefined;
	isLoading: boolean;
	username: string;
	onUserSelect: (username: string) => void;
	onTimeWindowChange: (hours: number) => void;
	selectedHours: number;
}

const TIME_WINDOW_OPTIONS = [24, 48, 72] as const;

export const OperatorDiagnosticPageView: FC<
	OperatorDiagnosticPageViewProps
> = ({
	data,
	isLoading,
	username,
	onUserSelect,
	onTimeWindowChange,
	selectedHours,
}) => {
	return (
		<Margins className="pb-12">
			<Breadcrumb>
				<BreadcrumbList>
					<BreadcrumbItem>
						<BreadcrumbLink asChild>
							<Link to="/connectionlog">Connections</Link>
						</BreadcrumbLink>
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbPage>Diagnostics: {username}</BreadcrumbPage>
					</BreadcrumbItem>
				</BreadcrumbList>
			</Breadcrumb>

			<PageHeader>
				<PageHeaderTitle>Connection Diagnostics</PageHeaderTitle>
				<PageHeaderSubtitle>{username}</PageHeaderSubtitle>
			</PageHeader>

			{/* Toolbar: user selector + time window */}
			<div className="flex items-center justify-between mb-4">
				<UserSelector currentUsername={username} onSelect={onUserSelect} />
				<div className="flex items-center gap-1">
					{TIME_WINDOW_OPTIONS.map((h) => (
						<Button
							key={h}
							size="sm"
							variant={h === selectedHours ? "default" : "outline"}
							onClick={() => onTimeWindowChange(h)}
						>
							{h}h
						</Button>
					))}
				</div>
			</div>

			{/* Stats bar */}
			{data && (
				<DiagnosticSummaryBar
					summary={data.summary}
					timeWindow={data.time_window}
				/>
			)}

			{/* Pattern banner */}
			{data && data.patterns.length > 0 && (
				<div className="mt-4">
					<PatternBanner patterns={data.patterns} />
				</div>
			)}

			{/* Loading */}
			{isLoading && (
				<div className="flex flex-col gap-4">
					<Skeleton className="h-16 w-full" />
					<Skeleton className="h-8 w-3/4" />
					<Skeleton className="h-40 w-full" />
				</div>
			)}

			{/* Session list */}
			{data && (
				<div className="mt-4">
					<SessionList workspaces={data.workspaces} />
				</div>
			)}
		</Margins>
	);
};
