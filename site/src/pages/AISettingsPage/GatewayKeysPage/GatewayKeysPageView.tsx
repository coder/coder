import { PlusIcon, TrashIcon } from "lucide-react";
import type { FC } from "react";
import type { AIGatewayKey } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
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
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { docs } from "#/utils/docs";
import { relativeTime } from "#/utils/time";

interface GatewayKeysPageViewProps {
	keys: AIGatewayKey[];
	isLoading: boolean;
	error: unknown;
	showPaywall: boolean;
	onCreateKey: () => void;
	onDeleteKey: (key: AIGatewayKey) => void;
}

export const GatewayKeysPageView: FC<GatewayKeysPageViewProps> = ({
	keys,
	isLoading,
	error,
	showPaywall,
	onCreateKey,
	onDeleteKey,
}) => {
	return (
		<div>
			<SettingsHeader
				actions={
					!showPaywall && (
						<Button variant="outline" onClick={onCreateKey}>
							<PlusIcon />
							Create key
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>AI Gateway Keys</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Keys authenticate standalone AI Gateway replicas to this deployment.
					The key value is shown only once when created.{" "}
					<Link href={docs("/ai-coder/ai-gateway")}>View docs</Link>
				</SettingsHeaderDescription>
			</SettingsHeader>

			{showPaywall && (
				<PaywallPremium
					message="AI Gateway"
					description="Authenticate standalone AI Gateway replicas to your deployment. You need a Premium license with AI Gateway enabled to use this feature."
					documentationLink={docs("/ai-coder/ai-gateway")}
				/>
			)}

			{!showPaywall && Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}

			{!showPaywall && (
				<Table aria-label="AI Gateway keys">
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Key prefix</TableHead>
							<TableHead>Last heartbeat</TableHead>
							<TableHead>Created</TableHead>
							<TableHead className="w-8" />
						</TableRow>
					</TableHeader>
					<TableBody size="lg">
						{isLoading ? (
							<TableLoader />
						) : error ? (
							<TableEmpty message="Failed to fetch AI Gateway keys" />
						) : keys.length === 0 ? (
							<TableEmpty
								message="No AI Gateway keys"
								description="Create a key to authenticate a standalone AI Gateway replica."
								cta={
									<Button variant="outline" onClick={onCreateKey}>
										<PlusIcon />
										Create key
									</Button>
								}
							/>
						) : (
							keys.map((key) => (
								<TableRow key={key.id}>
									<TableCell>{key.name}</TableCell>
									<TableCell>
										<span className="font-mono text-content-secondary">
											{key.key_prefix}
										</span>
									</TableCell>
									<TableCell>
										{key.last_heartbeat_at ? (
											<span className="block first-letter:uppercase">
												{relativeTime(new Date(key.last_heartbeat_at))}
											</span>
										) : (
											<span className="text-content-disabled">Never</span>
										)}
									</TableCell>
									<TableCell>
										<span className="block first-letter:uppercase">
											{relativeTime(new Date(key.created_at))}
										</span>
									</TableCell>
									<TableCell>
										<Button
											variant="destructive"
											size="icon"
											aria-label={`Delete ${key.name}`}
											onClick={() => onDeleteKey(key)}
										>
											<TrashIcon className="size-icon-sm" />
										</Button>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			)}
		</div>
	);
};
