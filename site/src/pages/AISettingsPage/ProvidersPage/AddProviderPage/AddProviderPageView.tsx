import { ArrowLeftIcon } from "lucide-react";
import { useLayoutEffect } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createAIProviderMutation } from "#/api/queries/aiProviders";
import type { Organization } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { ProviderForm } from "../components/ProviderForm";
import { providerFormValuesToCreate } from "../components/providerFormApiMap";

const ORGANIZATION_QUERY_PARAM = "organizationId";

interface AddProviderPageViewProps {
	organizations: Organization[] | undefined;
}

const AddProviderPageView: React.FC<AddProviderPageViewProps> = ({
	organizations,
}) => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createMutation = useMutation(createAIProviderMutation(queryClient));
	const [searchParams, setSearchParams] = useSearchParams();
	// TODO: AI providers are not yet organization-scoped on the wire, so the
	// selected org is round-tripped in the URL only. When the server supports
	// scoping, pass this id through providerFormValuesToCreate / the create
	// mutation.
	const selectedOrganizationId = searchParams.get(ORGANIZATION_QUERY_PARAM);

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

	const backHref = selectedOrganizationId
		? `/ai/settings?${ORGANIZATION_QUERY_PARAM}=${encodeURIComponent(selectedOrganizationId)}`
		: "/ai/settings";

	const successHref = (providerName: string) =>
		selectedOrganizationId
			? `/ai/settings/${providerName}?${ORGANIZATION_QUERY_PARAM}=${encodeURIComponent(selectedOrganizationId)}`
			: `/ai/settings/${providerName}`;

	return (
		<>
			<div className="pt-4 px-6">
				<Link to={backHref}>
					<Button variant="subtle">
						<ArrowLeftIcon />
						<span>Back to providers</span>
					</Button>
				</Link>
			</div>
			<div className="mx-auto w-full max-w-screen-sm flex flex-col gap-6">
				<PageHeader className="pt-6 pb-0">
					<PageHeaderTitle>Add a provider</PageHeaderTitle>
					<PageHeaderSubtitle>
						Connect third-party LLM services like OpenAI, Anthropic, or Amazon
						Bedrock. Each provider supplies models that users can select for
						their conversations.
					</PageHeaderSubtitle>
				</PageHeader>
				<div className="border border-solid p-6 rounded-lg">
					<ProviderForm
						editing={false}
						isLoading={createMutation.isPending}
						submitError={createMutation.error}
						organizations={organizations}
						selectedOrganizationId={selectedOrganizationId ?? ""}
						onOrganizationChange={handleSelectOrganization}
						onSubmit={(values) => {
							const request = providerFormValuesToCreate(values);
							createMutation.mutate(request, {
								onSuccess: (res) => {
									toast.success(
										`Provider "${res.display_name || res.name}" added.`,
									);
									void navigate(successHref(res.name));
								},
								onError: (error) => {
									const name = values.name.trim();
									toast.error(
										getErrorMessage(
											error,
											name
												? `Failed to add provider "${name}".`
												: "Failed to add provider.",
										),
									);
								},
							});
						}}
					/>
				</div>
			</div>
		</>
	);
};

export default AddProviderPageView;
