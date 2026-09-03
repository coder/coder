import { ArrowLeftIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { postApp } from "#/api/queries/oauth2";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { OAuth2AppForm } from "./OAuth2AppForm";

const BACK_HREF = "/deployment/oauth2-provider/apps";

export const CreateOAuth2AppPageView: FC = () => {
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const postAppMutation = useMutation(postApp(queryClient));

	const defaultValues = {
		name: searchParams.get("name") ?? "",
		callback_url: searchParams.get("callback_url") ?? "",
		icon: searchParams.get("icon") ?? "",
	};
	const [icon, setIcon] = useState(defaultValues.icon);

	return (
		<>
			<title>{pageTitle("Add an OAuth2 application")}</title>

			<Button variant="subtle" asChild className="-ml-3">
				<Link to={BACK_HREF}>
					<ArrowLeftIcon />
					<span>Back to applications</span>
				</Link>
			</Button>

			<div className="flex flex-col gap-6 pt-6">
				<div className="flex items-center gap-4 min-w-0">
					<Avatar variant="icon" size="lg" src={icon} fallback="App" />
					<SettingsHeaderTitle>Add an OAuth2 application</SettingsHeaderTitle>
				</div>
				<p className="text-sm text-content-secondary m-0">
					Configure an application to use Coder as an OAuth2 provider.
				</p>

				<div className="border border-solid p-6 rounded-lg">
					<OAuth2AppForm
						onSubmit={async (req) => {
							try {
								const app = await postAppMutation.mutateAsync(req);
								toast.success(
									`OAuth2 application "${app.name}" created successfully.`,
								);
								// Awaited so the form's submitting state stays true through
								// navigation, keeping the unsaved-changes prompt suppressed.
								await navigate(
									`/deployment/oauth2-provider/apps/${app.id}?created=true`,
								);
							} catch (error) {
								toast.error(
									getErrorMessage(
										error,
										req.name.trim()
											? `Failed to create "${req.name}" OAuth2 application.`
											: "Failed to create OAuth2 application.",
									),
									{ description: getErrorDetail(error) },
								);
							}
						}}
						isUpdating={postAppMutation.isPending}
						error={postAppMutation.error}
						defaultValues={defaultValues}
						disabled={!permissions.createOAuth2App}
						onIconChange={setIcon}
					/>
				</div>
			</div>
		</>
	);
};
