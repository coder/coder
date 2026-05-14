import { ArrowDownIcon, ArrowUpIcon, EllipsisVerticalIcon, SearchIcon } from "lucide-react";
import { type FC, useState } from "react";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { Avatar } from "#/components/Avatar/Avatar";
import { Input } from "#/components/Input/Input";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";

interface GroupRow {
	name: string;
	subtitle: string;
	avatar: string;
	costs: string;
	inputTokens: string;
	outputTokens: string;
	cacheRead: string;
	cacheWrite: string;
}

const MOCK_GROUPS: GroupRow[] = [
	{
		name: "Devops",
		subtitle: "5 Users",
		avatar: "🍌",
		costs: "$173,000 USD",
		inputTokens: "2,856",
		outputTokens: "1.6M",
		cacheRead: "542.7M",
		cacheWrite: "38.9M",
	},
	{
		name: "Some-Group",
		subtitle: "sub entry",
		avatar: "🍕",
		costs: "$173,000 USD",
		inputTokens: "2,856",
		outputTokens: "1.6M",
		cacheRead: "354.6M",
		cacheWrite: "12.1M",
	},
	{
		name: "Cell text",
		subtitle: "sub entry",
		avatar: "",
		costs: "$173,000 USD",
		inputTokens: "2,856",
		outputTokens: "1.6M",
		cacheRead: "264.9M",
		cacheWrite: "11.3M",
	},
	{
		name: "Cell text",
		subtitle: "sub entry",
		avatar: "",
		costs: "$173,000 USD",
		inputTokens: "2,856",
		outputTokens: "1.6M",
		cacheRead: "654.7M",
		cacheWrite: "46.8M",
	},
];

const AISettingsUsageStatsPage: FC = () => {
	const [activeTab, setActiveTab] = useState("groups");

	return (
		<div>
			<div className="flex items-start justify-between mb-6">
				<div>
					<h1 className="text-3xl font-semibold mt-0 mb-2">AI usage</h1>
					<p className="text-content-secondary text-sm mt-0">
						Monitor spend and usage across your deployment.
					</p>
				</div>
			</div>

			<Tabs value={activeTab} onValueChange={setActiveTab}>
				<TabsList>
					<TabsTrigger value="groups">Groups</TabsTrigger>
					<TabsTrigger value="users">Users</TabsTrigger>
				</TabsList>

				<TabsContent value="groups">
					<div className="flex items-center justify-between mt-6 mb-6">
						<div className="relative w-80">
							<SearchIcon className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-content-secondary" />
							<Input placeholder="Search groups.." className="pl-10" />
						</div>
						<span className="text-sm text-content-secondary">
							Mar 9, 2026 → Apr 8 2026
						</span>
					</div>

					<Table aria-label="AI usage by group">
						<TableHeader>
							<TableRow>
								<TableHead>Name</TableHead>
								<TableHead>Costs</TableHead>
								<TableHead>Input / output</TableHead>
								<TableHead>Cache read</TableHead>
								<TableHead>Cache write</TableHead>
								<TableHead className="w-10" />
							</TableRow>
						</TableHeader>
						<TableBody>
							{MOCK_GROUPS.map((group, i) => (
								<TableRow key={`${group.name}-${i}`}>
									<TableCell>
										<div className="flex items-center gap-3">
											<Avatar
												size="md"
												fallback={group.name}
												src={group.avatar || undefined}
											/>
											<div>
												<div className="text-sm font-medium text-content-primary">
													{group.name}
												</div>
												<div className="text-xs text-content-secondary">
													{group.subtitle}
												</div>
											</div>
										</div>
									</TableCell>
									<TableCell>
										<span className="text-sm text-content-secondary">
											{group.costs}
										</span>
									</TableCell>
									<TableCell>
										<div className="flex items-center gap-2">
											<span className="inline-flex items-center gap-1 rounded-md bg-surface-secondary px-2 py-0.5 text-xs text-content-secondary">
												<ArrowDownIcon className="size-3" />
												{group.inputTokens}
											</span>
											<span className="inline-flex items-center gap-1 rounded-md bg-surface-secondary px-2 py-0.5 text-xs text-content-secondary">
												<ArrowUpIcon className="size-3" />
												{group.outputTokens}
											</span>
										</div>
									</TableCell>
									<TableCell>
										<span className="text-sm text-content-primary">
											{group.cacheRead}
										</span>
									</TableCell>
									<TableCell>
										<span className="text-sm text-content-primary">
											{group.cacheWrite}
										</span>
									</TableCell>
									<TableCell>
										<button
											type="button"
											className="flex items-center justify-center w-8 h-8 rounded-md bg-transparent border-none cursor-pointer hover:bg-surface-secondary"
										>
											<EllipsisVerticalIcon className="size-4 text-content-secondary" />
										</button>
									</TableCell>
								</TableRow>
							))}
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
							Showing <strong className="text-content-primary">1</strong> to <strong className="text-content-primary">4</strong> of <strong className="text-content-primary">4</strong> groups
						</span>
					</div>
				</TabsContent>

				<TabsContent value="users">
					<div className="mt-6 text-sm text-content-secondary">
						User usage data will appear here.
					</div>
				</TabsContent>
			</Tabs>
		</div>
	);
};

export default AISettingsUsageStatsPage;
