import { ArrowLeftIcon } from "lucide-react";
import { useMutation, useQueryClient } from "react-query";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createAIProviderMutation } from "#/api/queries/aiProviders";
import type { AIProviderType } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { findAddableProvider } from "../components/addableProviderTypes";
import { ProviderForm } from "../components/ProviderForm";
import { providerFormValuesToCreate } from "../components/providerFormApiMap";

interface AddProviderPageViewProps {
	/**
	 * Pre-selected provider type passed from the parent page after it
	 * validates the `?type=` URL parameter against the supported list.
	 * The form is type-locked to this value; users return to the
	 * providers list and reopen the dropdown to switch type.
	 */
	type: AIProviderType;
}

const AddProviderPageView: React.FC<AddProviderPageViewProps> = ({ type }) => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createMutation = useMutation(createAIProviderMutation(queryClient));
	const provider = findAddableProvider(type);
	const providerLabel = provider?.label ?? type;

	return (
		<>
			<div className="pt-4 px-6">
				<Link to="/ai/settings">
					<Button variant="subtle">
						<ArrowLeftIcon />
						<span>Back to providers</span>
					</Button>
				</Link>
			</div>
			<div className="mx-auto w-full max-w-screen-sm flex flex-col gap-6">
				<PageHeader className="pt-6 pb-0">
					<PageHeaderTitle>{`Add ${providerLabel} provider`}</PageHeaderTitle>
					<PageHeaderSubtitle>
						Configure connection details and credentials for this provider. The
						provider supplies models that users can select for their
						conversations.
					</PageHeaderSubtitle>
				</PageHeader>
				<div className="border border-solid p-6 rounded-lg">
					<ProviderForm
						editing={false}
						initialValues={{ type }}
						isLoading={createMutation.isPending}
						submitError={createMutation.error}
						onSubmit={(values) => {
							const request = providerFormValuesToCreate(values);
							createMutation.mutate(request, {
								onSuccess: (res) => {
									toast.success(
										`Provider "${res.display_name || res.name}" added.`,
									);
									void navigate(`/ai/settings/${res.name}`);
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
