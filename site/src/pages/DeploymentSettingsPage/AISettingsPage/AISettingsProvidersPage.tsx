import { Check, ChevronRight, Plus, X } from "lucide-react";
import type { FC } from "react";
import { useNavigate } from "react-router";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";

interface ProviderRow {
	name: string;
	models: number;
	baseUrl: string;
	userKeys: boolean;
}

const MOCK_PROVIDERS: ProviderRow[] = [
	{
		name: "Anthropic",
		models: 3,
		baseUrl: "https://api.anthropic.com/",
		userKeys: true,
	},
	{
		name: "OpenAI",
		models: 2,
		baseUrl: "https://api.openai.com/v1",
		userKeys: true,
	},
	{
		name: "AWS Bedrock",
		models: 1,
		baseUrl: "https://bedrock-runtime.us-east-2.amazonaws.com",
		userKeys: false,
	},
	{
		name: "Vercel AI Gateway",
		models: 1,
		baseUrl: "https://ai-gateway.vercel.sh/v1/",
		userKeys: false,
	},
];

const AISettingsProvidersPage: FC = () => {
	const navigate = useNavigate();

	return (
		<div>
			<div className="flex items-start justify-between mb-6">
				<div>
					<h1 className="text-3xl font-semibold mt-0 mb-2">Providers</h1>
					<p className="text-content-secondary text-sm mt-0">
						Connect third-party LLM services like OpenAI, Anthropic, or Google.
						Each provider supplies models that users can select for their
						conversations.
					</p>
				</div>
				<Button variant="default">
					<Plus />
					Add provider
				</Button>
			</div>

			<Table aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Base URL</TableHead>
						<TableHead>User keys</TableHead>
						<TableHead className="w-10" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{MOCK_PROVIDERS.map((provider) => (
						<TableRow
							key={provider.name}
							hover
							className="cursor-pointer"
							onClick={() => navigate(`/deployment/ai-settings/providers/${provider.name.toLowerCase().replace(/\s+/g, "-")}`)}
						>
							<TableCell>
								<div className="flex items-center gap-3">
									<Avatar size="md" fallback={provider.name} />
									<div>
										<div className="text-sm font-medium text-content-primary">
											{provider.name}
										</div>
										<div className="text-xs text-content-secondary">
											{provider.models} models
										</div>
									</div>
								</div>
							</TableCell>
							<TableCell>
								<span className="text-sm text-content-secondary">
									{provider.baseUrl}
								</span>
							</TableCell>
							<TableCell>
								{provider.userKeys ? (
									<Check className="size-4 text-content-success" />
								) : (
									<X className="size-4 text-content-secondary" />
								)}
							</TableCell>
							<TableCell>
								<ChevronRight className="size-4 text-content-secondary" />
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
					Showing <strong className="text-content-primary">1</strong> to <strong className="text-content-primary">4</strong> of <strong className="text-content-primary">4</strong> providers
				</span>
			</div>
		</div>
	);
};

export default AISettingsProvidersPage;
