import { PlusIcon } from "lucide-react";
import { Link, useNavigate } from "react-router";
import type { AIProvider } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { ProviderRow } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderRow";

interface ProvidersPageViewProps {
	isLoading: boolean;
	isFetching: boolean;
	providers: AIProvider[];
}

const ProvidersPageView: React.FC<ProvidersPageViewProps> = ({
	isLoading,
	isFetching,
	providers,
}) => {
	const navigate = useNavigate();

	return (
		<>
			<PageHeader
				className="pt-4 pb-8"
				actions={
					<Link to="/ai/settings/add">
						<Button>
							<PlusIcon />
							<span>Add provider</span>
						</Button>
					</Link>
				}
			>
				<PageHeaderTitle>Providers</PageHeaderTitle>
				<PageHeaderSubtitle>
					Connect third-party LLM services like OpenAI, Anthropic, or Amazon
					Bedrock. Each provider supplies models that users can select for their
					conversations.
				</PageHeaderSubtitle>
			</PageHeader>
			<Table className="table-fixed" aria-label="AI providers">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Base URL</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading || isFetching ? (
						<TableLoader />
					) : providers.length === 0 ? (
						<TableEmpty message="No providers available" />
					) : (
						providers.map((provider) => (
							<ProviderRow
								key={provider.name}
								provider={provider}
								onClick={() => navigate(`/ai/settings/${provider.name}`)}
							/>
						))
					)}
				</TableBody>
			</Table>
		</>
	);
};

export default ProvidersPageView;
