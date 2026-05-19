import { PlusIcon } from "lucide-react";
import { useLayoutEffect } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import type { AIProvider, Organization } from "#/api/typesGenerated";
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
import { OrganizationPicker } from "#/pages/AISettingsPage/ProvidersPage/components/OrganizationPicker";
import { ProviderRow } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderRow";

interface ProvidersPageViewProps {
	isLoading: boolean;
	isFetching: boolean;
	providers: AIProvider[];
	organizations: Organization[] | undefined;
}

const ORGANIZATION_QUERY_PARAM = "organizationId";

const ProvidersPageView: React.FC<ProvidersPageViewProps> = ({
	isLoading,
	isFetching,
	providers,
	organizations,
}) => {
	const navigate = useNavigate();
	// TODO: GET /api/v2/ai/providers does not yet accept an organization
	// filter, so the selected org only drives URL state today. Once the
	// server supports scoping, plumb `selectedOrganizationId` through the
	// aiProvidersList query.
	const [searchParams, setSearchParams] = useSearchParams();
	const selectedOrganizationId = searchParams.get(ORGANIZATION_QUERY_PARAM);

	// Default the picker to the first org when the URL has no
	// `organizationId` (or points at one the user can no longer see). We use
	// useLayoutEffect so the URL update happens in the same paint as the
	// initial render and downstream pages reading the param see a stable
	// value.
	useLayoutEffect(() => {
		if (!organizations?.length) {
			return;
		}
		const present =
			selectedOrganizationId !== null &&
			organizations.some((o) => o.id === selectedOrganizationId);
		if (present) {
			return;
		}
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				next.set(ORGANIZATION_QUERY_PARAM, organizations[0].id);
				return next;
			},
			{ replace: true },
		);
	}, [organizations, selectedOrganizationId, setSearchParams]);

	const handleSelectOrganization = (id: string) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				next.set(ORGANIZATION_QUERY_PARAM, id);
				return next;
			},
			{ replace: true },
		);
	};

	const addHref = selectedOrganizationId
		? `/ai/settings/add?${ORGANIZATION_QUERY_PARAM}=${encodeURIComponent(selectedOrganizationId)}`
		: "/ai/settings/add";

	return (
		<>
			<PageHeader
				className="pt-4 pb-8"
				actions={
					<>
						<OrganizationPicker
							organizations={organizations}
							value={selectedOrganizationId ?? ""}
							onValueChange={handleSelectOrganization}
						/>
						<Link to={addHref}>
							<Button>
								<PlusIcon />
								<span>Add provider</span>
							</Button>
						</Link>
					</>
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
								onClick={() =>
									navigate(
										selectedOrganizationId
											? `/ai/settings/${provider.name}?${ORGANIZATION_QUERY_PARAM}=${encodeURIComponent(selectedOrganizationId)}`
											: `/ai/settings/${provider.name}`,
									)
								}
							/>
						))
					)}
				</TableBody>
			</Table>
		</>
	);
};

export default ProvidersPageView;
