import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { Link } from "react-router-dom";
import { OAuth2AppForm } from "./OAuth2AppForm";
import { Button } from "components/Button/Button";
import { ChevronLeftIcon } from "lucide-react";

type CreateOAuth2AppProps = {
	isUpdating: boolean;
	createApp: (req: TypesGen.PostOAuth2ProviderAppRequest) => void;
	error?: unknown;
};

export const CreateOAuth2AppPageView: FC<CreateOAuth2AppProps> = ({
	isUpdating,
	createApp,
	error,
}) => {
	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="Add an OAuth2 application"
					description="Configure an application to use Coder as an OAuth2 provider."
				/>
				<Button variant="outline" asChild>
					<Link to="/deployment/oauth2-provider/apps">
						<ChevronLeftIcon />
						All OAuth2 Applications
					</Link>
				</Button>
			</Stack>

			<Stack>
				{error ? <ErrorAlert error={error} /> : undefined}
				<OAuth2AppForm
					onSubmit={createApp}
					isUpdating={isUpdating}
					error={error}
				/>
			</Stack>
		</>
	);
};
