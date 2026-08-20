import { KeyRoundIcon, PlusIcon, ShieldCheckIcon } from "lucide-react";
import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { CopyableValue } from "#/components/CopyableValue/CopyableValue";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { ClientTypeBadge, type OAuth2ClientType } from "./ClientTypeBadge";

export type OAuth2ClientSecret = {
	id: string;
	/** Truncated secret, e.g. `••••••••3f2a`. The full value is shown once. */
	preview: string;
	lastUsedAt?: string;
};

type OAuth2ClientDetailViewProps = {
	name: string;
	type: OAuth2ClientType;
	clientId: string;
	callbackUrl: string;
	/** Only meaningful for confidential clients. */
	secrets?: readonly OAuth2ClientSecret[];
	/** Shown once, immediately after generation. */
	newSecret?: string;
	onGenerateSecret?: () => void;
	onDeleteSecret?: (secret: OAuth2ClientSecret) => void;
};

/**
 * Settings for a single OAuth2 client (Flow 3 — PLAT-504).
 *
 * The point of the screen: a public client has no client secret, so the secret
 * section is **absent** rather than present-and-empty or present-and-disabled.
 * A disabled secret field still tells the admin a secret exists somewhere, and
 * an empty one invites them to look for the generate button. Neither is true.
 *
 * In its place, the credentials section states what the client authenticates
 * with instead, so the absence reads as a deliberate answer rather than as
 * something that failed to load.
 */
export const OAuth2ClientDetailView: FC<OAuth2ClientDetailViewProps> = ({
	name,
	type,
	clientId,
	callbackUrl,
	secrets = [],
	newSecret,
	onGenerateSecret,
	onDeleteSecret,
}) => {
	const isPublic = type === "public";

	return (
		<div className="flex flex-col gap-8 max-w-3xl">
			<div className="flex flex-col gap-2">
				<SettingsHeader>
					<SettingsHeaderTitle>{name}</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						How this application authenticates and where Coder sends users back
						to.
					</SettingsHeaderDescription>
				</SettingsHeader>
				<div className="flex items-center gap-2">
					<ClientTypeBadge type={type} />
					<span className="text-xs text-content-secondary">
						{isPublic
							? "Runs on a user's machine — authenticates with PKCE"
							: "Runs on a server — authenticates with a client secret"}
					</span>
				</div>
			</div>

			<section className="flex flex-col gap-4">
				<h2 className="m-0 text-base font-semibold">Details</h2>
				<dl className="m-0 grid grid-cols-[160px_1fr] gap-x-4 gap-y-3">
					<dt className="text-sm text-content-secondary">Client ID</dt>
					<dd className="m-0 min-w-0">
						<CopyableValue value={clientId} className="font-mono text-sm">
							{clientId}
						</CopyableValue>
					</dd>
					<dt className="text-sm text-content-secondary">Callback URL</dt>
					<dd className="m-0 min-w-0 truncate font-mono text-sm text-content-primary">
						{callbackUrl}
					</dd>
				</dl>
			</section>

			<section className="flex flex-col gap-4">
				<div className="flex items-center justify-between gap-4">
					<h2 className="m-0 text-base font-semibold">
						{isPublic ? "Authentication" : "Client secrets"}
					</h2>
					{!isPublic && onGenerateSecret && (
						<Button size="sm" variant="outline" onClick={onGenerateSecret}>
							<PlusIcon />
							Generate secret
						</Button>
					)}
				</div>

				{isPublic ? (
					<div className="flex items-start gap-3 rounded-md border border-solid border-border p-4">
						<ShieldCheckIcon
							aria-hidden="true"
							className="mt-0.5 size-icon-sm shrink-0 text-content-secondary"
						/>
						<div className="flex flex-col gap-1">
							<span className="text-sm text-content-primary">
								This application uses PKCE
							</span>
							<span className="text-xs text-content-secondary">
								Public clients can't keep a secret — anything shipped to a
								user's machine can be read out of it. Instead, each
								authorization request carries a one-time proof key that only
								that request can redeem, so there's no long-lived credential to
								store or rotate here.
							</span>
						</div>
					</div>
				) : (
					<>
						{newSecret && (
							<Alert severity="warning" prominent>
								<div className="flex flex-col gap-2">
									<span>Copy this secret now. It won't be shown again.</span>
									<CopyableValue
										value={newSecret}
										className="font-mono text-sm break-all"
									>
										{newSecret}
									</CopyableValue>
								</div>
							</Alert>
						)}

						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Secret</TableHead>
									<TableHead className="w-48">Last used</TableHead>
									<TableHead className="w-24">
										<span className="sr-only">Actions</span>
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{secrets.length === 0 ? (
									<TableEmpty
										message="No client secrets"
										description="This application can't authenticate until it has one."
										cta={
											onGenerateSecret ? (
												<Button onClick={onGenerateSecret}>
													<KeyRoundIcon />
													Generate secret
												</Button>
											) : undefined
										}
									/>
								) : (
									secrets.map((secret) => (
										<TableRow key={secret.id}>
											<TableCell className="font-mono text-sm">
												{secret.preview}
											</TableCell>
											<TableCell className="text-sm text-content-secondary">
												{secret.lastUsedAt ?? "Never"}
											</TableCell>
											<TableCell className="text-right">
												<Button
													size="sm"
													variant="outline"
													onClick={() => onDeleteSecret?.(secret)}
												>
													Delete
												</Button>
											</TableCell>
										</TableRow>
									))
								)}
							</TableBody>
						</Table>
					</>
				)}
			</section>
		</div>
	);
};
