import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	templateAIEgressPolicy,
	updateTemplateAIEgressPolicy,
} from "#/api/queries/templates";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateAIEgressPolicyPageView } from "./TemplateAIEgressPolicyPageView";

const TemplateAIEgressPolicyPage: FC = () => {
	const queryClient = useQueryClient();
	const { template, permissions } = useTemplateSettings();
	const policyQuery = useQuery(templateAIEgressPolicy(template.id));
	const updatePolicyMutation = useMutation(
		updateTemplateAIEgressPolicy(template.id, queryClient),
	);

	if (!policyQuery.data) {
		if (policyQuery.isError) {
			return <ErrorAlert error={policyQuery.error} />;
		}
		return <Loader label="Loading AI egress policy" />;
	}

	return (
		<>
			<title>{pageTitle(template.name, "AI Egress Policy")}</title>
			<TemplateAIEgressPolicyPageView
				policy={policyQuery.data}
				canUpdate={permissions.canUpdateTemplate}
				isFetching={policyQuery.isFetching}
				isSubmitting={updatePolicyMutation.isPending}
				loadError={policyQuery.isError ? policyQuery.error : undefined}
				submitError={updatePolicyMutation.error}
				onSubmit={(request, onSuccess) => {
					updatePolicyMutation.mutate(request, {
						onSuccess: (policy) => {
							onSuccess(policy);
							toast.success("AI egress policy updated successfully.");
						},
					});
				}}
			/>
		</>
	);
};

export default TemplateAIEgressPolicyPage;
