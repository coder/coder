import { PlusIcon, RefreshCwIcon } from "lucide-react";
import { type FC, useState } from "react";
import type {
	CreateUserSecretRequest,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";
import { Link } from "#/components/Link/Link";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { docs } from "#/utils/docs";
import { SecretDialog } from "./SecretDialog";
import { SecretsTable } from "./SecretsTable";

type SecretsPageViewProps = {
	secrets?: readonly UserSecret[];
	isLoading: boolean;
	hasLoaded: boolean;
	isRefreshing: boolean;
	isCreating: boolean;
	isUpdating: boolean;
	isDeleting: boolean;
	getSecretsError?: unknown;
	onRefresh: () => void;
	onCreateSecret: (
		request: CreateUserSecretRequest,
	) => Promise<unknown> | unknown;
	onUpdateSecret: (
		name: string,
		request: UpdateUserSecretRequest,
	) => Promise<unknown> | unknown;
	onDeleteSecret: (secret: UserSecret) => void;
};

export const SecretsPageView: FC<SecretsPageViewProps> = ({
	secrets = [],
	isLoading,
	hasLoaded,
	isRefreshing,
	isCreating,
	isUpdating,
	isDeleting,
	getSecretsError,
	onRefresh,
	onCreateSecret,
	onUpdateSecret,
	onDeleteSecret,
}) => {
	const [dialogSecret, setDialogSecret] = useState<UserSecret | null>();
	const isDialogOpen = dialogSecret !== undefined;

	return (
		<div className="flex flex-col gap-6">
			<SettingsHeader
				actions={
					<div className="flex flex-wrap gap-2">
						<Button
							variant="outline"
							onClick={onRefresh}
							disabled={isRefreshing}
						>
							<Spinner loading={isRefreshing}>
								<RefreshCwIcon />
							</Spinner>
							Refresh
						</Button>
					</div>
				}
			>
				<SettingsHeaderTitle
					tooltip={<FeatureStageBadge contentType="early_access" size="md" />}
				>
					Secrets
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					When multiple secrets share the same environment variable or file
					path, you'll choose which one to use during workspace creation.{" "}
					<Link
						href={docs("/user-guides/user-secrets")}
						target="_blank"
						rel="noreferrer"
						showExternalIcon={false}
					>
						View docs
					</Link>
				</SettingsHeaderDescription>
			</SettingsHeader>

			<SecretDialog
				open={isDialogOpen}
				secret={dialogSecret ?? undefined}
				secrets={secrets}
				isSubmitting={isCreating || isUpdating}
				onClose={() => setDialogSecret(undefined)}
				onCreateSecret={onCreateSecret}
				onUpdateSecret={onUpdateSecret}
			/>

			{getSecretsError ? <ErrorAlert error={getSecretsError} /> : undefined}

			<section className="flex flex-col gap-4">
				<div className="flex items-center justify-between gap-4">
					<h2 className="m-0 text-xl font-semibold">Your secrets</h2>
					<Button onClick={() => setDialogSecret(null)}>
						<PlusIcon />
						Add secret
					</Button>
				</div>

				<SecretsTable
					secrets={secrets}
					isLoading={isLoading}
					hasLoaded={hasLoaded}
					isDeleting={isDeleting}
					onAddSecret={() => setDialogSecret(null)}
					onEditSecret={(secret) => setDialogSecret(secret)}
					onDeleteSecret={onDeleteSecret}
				/>
			</section>
		</div>
	);
};
