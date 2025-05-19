import type { ProvisionerDaemon, ProvisionerKey } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { CopyButton } from "components/CopyButton/CopyButton";
import {
	Table,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTags";
import { LastConnectionHead } from "pages/OrganizationSettingsPage/OrganizationProvisionersPage/LastConnectionHead";
import { ProvisionerRow } from "pages/OrganizationSettingsPage/OrganizationProvisionersPage/ProvisionerRow";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { relativeTime } from "utils/time";

type ProvisionerKeyRowProps = {
	readonly provisionerKey: ProvisionerKey;
	readonly provisioners: readonly ProvisionerDaemon[];
	readonly buildVersion: string | undefined;
	defaultIsOpen: boolean;
};

export const ProvisionerKeyRow: FC<ProvisionerKeyRowProps> = ({
	provisionerKey,
	provisioners,
	buildVersion,
	defaultIsOpen = false,
}) => {
	const [isOpen, setIsOpen] = useState(defaultIsOpen);

	return (
		<>
			<TableRow key={provisionerKey.id}>
				<TableCell>
					<Button
						variant="subtle"
						size="sm"
						className={cn([
							isOpen && "text-content-primary",
							"p-0 h-auto min-w-0 align-middle",
						])}
						onClick={() => setIsOpen((v) => !v)}
					>
						{isOpen ? <ChevronDownIcon /> : <ChevronRightIcon />}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						<span className="block first-letter:uppercase">
							{relativeTime(new Date(provisionerKey.created_at))}
						</span>
					</Button>
				</TableCell>
				<TableCell>{provisionerKey.name}</TableCell>
				<TableCell>
					<span className="font-mono text-content-primary">
						{provisionerKey.id}
					</span>
					<CopyButton text={provisionerKey.id} label="Copy ID" />
				</TableCell>
				<TableCell>
					{Object.entries(provisionerKey.tags).map(([k, v]) => (
						<span key={k}>
							<ProvisionerTag label={k} value={v} />
						</span>
					))}
				</TableCell>
				<TableCell>{provisioners.length}</TableCell>
			</TableRow>

			{isOpen && (
				<TableRow>
					<TableCell
						colSpan={999}
						className="p-0 border-l-4 border-accent bg-muted/50"
						style={{ paddingLeft: "1.5rem" }}
					>
						{provisioners.length === 0 ? (
							<TableRow>
								<TableCell colSpan={999} className="p-4 border-t-0">
									<span className="text-muted-foreground">
										No provisioners found for this key.
									</span>
								</TableCell>
							</TableRow>
						) : (
							<Table>
								<TableHeader>
									<TableRow>
										<TableHead>Name</TableHead>
										<TableHead>Key</TableHead>
										<TableHead>Version</TableHead>
										<TableHead>Status</TableHead>
										<TableHead>Tags</TableHead>
										<TableHead>
											<LastConnectionHead />
										</TableHead>
									</TableRow>
								</TableHeader>
								{provisioners.map((p) => (
									<ProvisionerRow
										key={p.id}
										buildVersion={buildVersion}
										provisioner={p}
										defaultIsOpen={false}
									/>
								))}
							</Table>
						)}
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
