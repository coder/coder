import { getErrorDetail } from "api/errors";
import { postApp } from "api/queries/oauth2";
import { useAuthenticated } from "hooks";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";

const CreateOAuth2AppPage: FC = () => {
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const postAppMutation = useMutation(postApp(queryClient));
	const canCreateApp = permissions.createOAuth2App;

	const defaultValues = {
		name: searchParams.get("name") ?? "",
		callback_url: searchParams.get("callback_url") ?? "",
		icon: searchParams.get("icon") ?? "",
	};

	return (
		<>
			<title>{pageTitle("New OAuth2 Application")}</title>

			<CreateOAuth2AppPageView
				isUpdating={postAppMutation.isPending}
				error={postAppMutation.error}
				defaultValues={defaultValues}
				createApp={async (req) => {
					const mutation = postAppMutation.mutateAsync(req, {
						onSuccess: (app) => {
							navigate(
								`/deployment/oauth2-provider/apps/${app.id}?created=true`,
							);
						},
					});
					toast.promise(mutation, {
						loading: `Creating OAuth2 application "${req.name}"...`,
						success: (app) =>
							`OAuth2 application "${app.name}" created successfully.`,
						error: (error) => ({
							message: `Failed to create "${req.name}" OAuth2 application.`,
							description: getErrorDetail(error),
						}),
					});
				}}
				canCreateApp={canCreateApp}
			/>
		</>
	);
};

export default CreateOAuth2AppPage;
