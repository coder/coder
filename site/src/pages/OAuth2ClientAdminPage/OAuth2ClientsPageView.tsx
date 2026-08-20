import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { ClientTypeBadge, type OAuth2ClientType } from "./ClientTypeBadge";

export type OAuth2ClientSummary = {
	id: string;
	name: string;
	type: OAuth2ClientType;
	callbackUrl: string;
};

type OAuth2ClientsPageViewProps = {
	clients?: readonly OAuth2ClientSummary[];
	isLoading?: boolean;
	canCreate?: boolean;
	onCreate: () => void;
	onSelect: (client: OAuth2ClientSummary) => void;
};

/**
 * Admin list of registered OAuth2 clients (Flow 3 — PLAT-504).
 *
 * Client type gets its own column rather than being tucked under the name: it
 * decides whether a client has a secret at all, and as its own column it can be
 * sorted and filtered later. Burying it as a subtitle would cost every
 * operation an admin might want to perform on it.
 */
export const OAuth2ClientsPageView: FC<OAuth2ClientsPageViewProps> = ({
	clients,
	isLoading = false,
	canCreate = true,
	onCreate,
	onSelect,
}) => {
	return (
		<div className="flex flex-col gap-6">
			<div className="flex items-start justify-between gap-4">
				<SettingsHeader>
					<SettingsHeaderTitle>OAuth2 applications</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Applications that can request access to this deployment on behalf of
						its users.
					</SettingsHeaderDescription>
				</SettingsHeader>
				{canCreate && (
					<Button onClick={onCreate}>
						<PlusIcon />
						Add application
					</Button>
				)}
			</div>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-32">Type</TableHead>
						<TableHead>Callback URL</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Open</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading && (
						<TableRow>
							<TableCell colSpan={4}>
								<Loader />
							</TableCell>
						</TableRow>
					)}

					{!isLoading && clients?.length === 0 && (
						<TableEmpty
							message="No applications yet"
							description="Register an application to let it request access to Coder on behalf of your users."
							cta={
								canCreate ? (
									<Button onClick={onCreate}>
										<PlusIcon />
										Add application
									</Button>
								) : undefined
							}
						/>
					)}

					{clients?.map((client) => (
						<TableRow key={client.id} className="cursor-pointer">
							<TableCell>
								<button
									type="button"
									className="border-none bg-transparent p-0 text-left text-sm text-content-primary"
									onClick={() => onSelect(client)}
								>
									{client.name}
								</button>
							</TableCell>
							<TableCell>
								<ClientTypeBadge type={client.type} />
							</TableCell>
							<TableCell className="min-w-0">
								{/* Truncated rather than wrapped; the full URL matters, so it's on a tooltip. */}
								<Tooltip>
									<TooltipTrigger className="block w-full truncate border-none bg-transparent p-0 text-left text-sm text-content-secondary">
										{client.callbackUrl}
									</TooltipTrigger>
									<TooltipContent>{client.callbackUrl}</TooltipContent>
								</Tooltip>
							</TableCell>
							<TableCell className="w-12 text-right">
								<ChevronRightIcon
									aria-hidden="true"
									className="size-icon-sm text-content-secondary"
								/>
							</TableCell>
						</TableRow>
					))}
				</TableBody>
			</Table>
		</div>
	);
};
