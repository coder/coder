import { ChevronRight } from "lucide-react";
import type { FC } from "react";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { cn } from "#/utils/cn";

type ModelStatus = "active" | "stale" | "disabled";

interface ModelRow {
	name: string;
	provider: string;
	providerIcon: string;
	cost: string;
	sessionShare: string;
	contextLimit: string;
	status: ModelStatus;
	statusDetail: string;
}

const MOCK_MODELS: ModelRow[] = [
	{
		name: "Opus 4.6 Max",
		provider: "Anthropic",
		providerIcon: "A",
		cost: "$379,031 USD",
		sessionShare: "87%",
		contextLimit: "1,000,000 tokens",
		status: "active",
		statusDetail: "In use, default",
	},
	{
		name: "claude-sonnet-4-6",
		provider: "Anthropic",
		providerIcon: "A",
		cost: "$73,470 USD",
		sessionShare: "11.48%",
		contextLimit: "200,000 tokens",
		status: "active",
		statusDetail: "5 min ago",
	},
	{
		name: "anthropic.claude-opus-4-6-v1",
		provider: "Anthropic",
		providerIcon: "C",
		cost: "$2,731 USD",
		sessionShare: "1.52%",
		contextLimit: "100,000 tokens",
		status: "stale",
		statusDetail: "27 days ago",
	},
	{
		name: "GPT 5.4 xhigh 1M",
		provider: "OpenAI",
		providerIcon: "G",
		cost: "$0 USD",
		sessionShare: "0%",
		contextLimit: "500,000 tokens",
		status: "disabled",
		statusDetail: "",
	},
];

const statusConfig: Record<
	ModelStatus,
	{ label: string; dotClass: string; textClass: string }
> = {
	active: {
		label: "Active",
		dotClass: "bg-content-success",
		textClass: "text-content-success",
	},
	stale: {
		label: "Stale",
		dotClass: "bg-orange-400",
		textClass: "text-orange-400",
	},
	disabled: {
		label: "Disabled",
		dotClass: "bg-content-disabled",
		textClass: "text-content-disabled",
	},
};

const AISettingsModelsPage: FC = () => {
	return (
		<div>
			<div className="flex items-start justify-between mb-6">
				<div>
					<h1 className="text-3xl font-semibold mt-0 mb-2">Models</h1>
					<p className="text-content-secondary text-sm mt-0 max-w-2xl">
						Choose which models from your configured providers are available for
						users to select. You can set a default and adjust context limits.
					</p>
				</div>
			</div>

			<Table aria-label="AI models">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Model cost</TableHead>
						<TableHead>Session share ⓘ</TableHead>
						<TableHead>Context limit</TableHead>
						<TableHead>Status</TableHead>
						<TableHead className="w-10" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{MOCK_MODELS.map((model) => {
						const cfg = statusConfig[model.status];
						return (
							<TableRow key={model.name}>
								<TableCell>
									<div className="flex items-center gap-3">
										<Avatar size="sm" fallback={model.providerIcon} />
										<span className="text-sm font-medium text-content-primary">
											{model.name}
										</span>
									</div>
								</TableCell>
								<TableCell>
									<span className="text-sm text-content-secondary">
										{model.cost}
									</span>
								</TableCell>
								<TableCell>
									<span className="text-sm text-content-primary">
										{model.sessionShare}
									</span>
								</TableCell>
								<TableCell>
									<span className="text-sm text-content-secondary">
										{model.contextLimit}
									</span>
								</TableCell>
								<TableCell>
									<div className="flex items-center gap-2">
										<span
											className={cn(
												"inline-block size-2 rounded-full",
												cfg.dotClass,
											)}
										/>
										<div>
											<div className={cn("text-sm font-medium", cfg.textClass)}>
												{cfg.label}
											</div>
											{model.statusDetail && (
												<div className="text-xs text-content-secondary">
													{model.statusDetail}
												</div>
											)}
										</div>
									</div>
								</TableCell>
								<TableCell>
									<ChevronRight className="size-4 text-content-secondary" />
								</TableCell>
							</TableRow>
						);
					})}
				</TableBody>
			</Table>

			<div className="relative mt-4 flex items-center justify-center">
				<PaginationWidgetBase
					currentPage={1}
					pageSize={4}
					totalRecords={24}
					onPageChange={() => {}}
				/>
				<span className="absolute right-0 text-sm text-content-secondary">
					Showing <strong className="text-content-primary">1</strong> to <strong className="text-content-primary">4</strong> of <strong className="text-content-primary">4</strong> models
				</span>
			</div>
		</div>
	);
};

export default AISettingsModelsPage;
