import type { ProvisionerDaemon, ProvisionerKey } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { CopyButton } from "components/CopyButton/CopyButton";
import { TableCell, TableRow } from "components/Table/Table";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTags";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import { cn } from "utils/cn";
import { relativeTime } from "utils/time";

type ProvisionerKeyRowProps = {
	readonly provisionerKey: ProvisionerKey;
	readonly provisioners: readonly ProvisionerDaemon[];
	defaultIsOpen: boolean;
};

export const ProvisionerKeyRow: FC<ProvisionerKeyRowProps> = ({
	provisionerKey,
	provisioners,
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
					<TableCell colSpan={999} className="p-4 border-t-0">
						{provisioners.length === 0 ? (
							<span className="text-muted-foreground">
								No provisioners found for this key.
							</span>
						) : (
							<dl>
								<dt>Provisioners:</dt>
								{provisioners.map((provisioner) => (
									<dd key={provisioner.id}>
										<span className="font-mono text-content-primary">
											{provisioner.name} ({provisioner.id}){" "}
										</span>
										<CopyButton
											text={provisioner.id}
											label="Copy provisioner ID"
										/>
										<Button size="xs" variant="outline" asChild>
											<RouterLink
												to={`../provisioners?${new URLSearchParams({ ids: provisioner.id })}`}
											>
												View provisioner
											</RouterLink>
										</Button>
									</dd>
								))}
							</dl>
						)}
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
