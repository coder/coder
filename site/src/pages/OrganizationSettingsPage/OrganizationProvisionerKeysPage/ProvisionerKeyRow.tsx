import type { ProvisionerDaemon, ProvisionerKey } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { TableCell, TableRow } from "components/Table/Table";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import {
	ProvisionerTag,
	ProvisionerTags,
	ProvisionerTruncateTags,
} from "modules/provisioners/ProvisionerTags";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
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
						{provisionerKey.name}
					</Button>
				</TableCell>
				<TableCell>
					{Object.entries(provisionerKey.tags).length > 0 ? (
						<ProvisionerTruncateTags tags={provisionerKey.tags} />
					) : (
						<span className="text-content-disabled">No tags</span>
					)}
				</TableCell>
				<TableCell>
					{provisioners.length > 0 ? (
						<TruncateProvisioners provisioners={provisioners} />
					) : (
						<span className="text-content-disabled">No provisioners</span>
					)}
				</TableCell>
				<TableCell>
					<span className="block first-letter:uppercase">
						{relativeTime(new Date(provisionerKey.created_at))}
					</span>
				</TableCell>
			</TableRow>

			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						<dl
							className={cn([
								"text-xs text-content-secondary",
								"m-0 grid grid-cols-[auto_1fr] gap-x-4 items-center",
								"[&_dd]:text-content-primary [&_dd]:font-mono [&_dd]:leading-[22px] [&_dt]:font-medium",
							])}
						>
							<dt>Creation time:</dt>
							<dd data-chromatic="ignore">{provisionerKey.created_at}</dd>

							<dt>Tags:</dt>
							<dd>
								<ProvisionerTags>
									{Object.entries(provisionerKey.tags).length === 0 && (
										<span className="text-content-disabled">No tags</span>
									)}
									{Object.entries(provisionerKey.tags).map(([key, value]) => (
										<ProvisionerTag key={key} label={key} value={value} />
									))}
								</ProvisionerTags>
							</dd>

							<dt>Provisioners:</dt>
							<dd>
								<ProvisionerTags>
									{provisioners.length === 0 && (
										<span className="text-content-disabled">
											No provisioners
										</span>
									)}
									{provisioners.map((provisioner) => (
										<Badge hover key={provisioner.id} size="sm" asChild>
											<RouterLink
												to={`../provisioners?${new URLSearchParams({ ids: provisioner.id })}`}
											>
												{provisioner.name}
											</RouterLink>
										</Badge>
									))}
								</ProvisionerTags>
							</dd>
						</dl>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};

type TruncateProvisionersProps = {
	provisioners: readonly ProvisionerDaemon[];
};

const TruncateProvisioners: FC<TruncateProvisionersProps> = ({
	provisioners,
}) => {
	const firstProvisioner = provisioners[0];
	const remainderCount = provisioners.length - 1;

	return (
		<ProvisionerTags>
			<Badge size="sm">{firstProvisioner!.name}</Badge>
			{remainderCount > 0 && <Badge size="sm">+{remainderCount}</Badge>}
		</ProvisionerTags>
	);
};
