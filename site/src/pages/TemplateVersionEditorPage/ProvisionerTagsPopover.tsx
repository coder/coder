import Link from "@mui/material/Link";
import type { ProvisionerDaemon } from "api/typesGenerated";
import { FormSection } from "components/Form/Form";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { ChevronDownIcon } from "lucide-react";
import { ProvisionerTagsField } from "modules/provisioners/ProvisionerTagsField";
import type { FC } from "react";
import { docs } from "utils/docs";

interface ProvisionerTagsPopoverProps {
	tags: ProvisionerDaemon["tags"];
	onTagsChange: (values: ProvisionerDaemon["tags"]) => void;
}

export const ProvisionerTagsPopover: FC<ProvisionerTagsPopoverProps> = ({
	tags,
	onTagsChange,
}) => {
	return (
		<Popover>
			<PopoverTrigger asChild>
				<TopbarButton color="neutral" className="!min-w-[28px]">
					<ChevronDownIcon className="size-icon-xs" />
					<span className="sr-only">Expand provisioner tags</span>
				</TopbarButton>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="w-[300px] bg-surface-secondary border-surface-quaternary"
			>
				<div className="text-content-secondary p-5 border-0 border-b border-solid border-border">
					<FormSection
						classes={{
							root: "flex flex-col gap-4",
						}}
						title="Provisioner Tags"
						description={
							<>
								Tags are a way to control which provisioner daemons complete
								which build jobs.&nbsp;
								<Link
									href={docs("/admin/provisioners")}
									target="_blank"
									rel="noreferrer"
								>
									Learn more...
								</Link>
							</>
						}
					>
						<ProvisionerTagsField value={tags} onChange={onTagsChange} />
					</FormSection>
				</div>
			</PopoverContent>
		</Popover>
	);
};
