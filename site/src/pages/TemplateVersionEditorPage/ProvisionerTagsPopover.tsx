import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Link from "@mui/material/Link";
import useTheme from "@mui/system/useTheme";
import type { ProvisionerDaemon } from "api/typesGenerated";
import { FormSection } from "components/Form/Form";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { ProvisionerTagsField } from "modules/provisioners/ProvisionerTagsField";
import type { FC } from "react";
import { docs } from "utils/docs";

export interface ProvisionerTagsPopoverProps {
	tags: ProvisionerDaemon["tags"];
	onTagsChange: (values: ProvisionerDaemon["tags"]) => void;
}

export const ProvisionerTagsPopover: FC<ProvisionerTagsPopoverProps> = ({
	tags,
	onTagsChange,
}) => {
	const theme = useTheme();

	return (
		<Popover>
			<PopoverTrigger>
				<TopbarButton
					color="neutral"
					css={{ paddingLeft: 0, paddingRight: 0, minWidth: "28px !important" }}
				>
					<ExpandMoreOutlined css={{ fontSize: 14 }} />
					<span className="sr-only">Expand provisioner tags</span>
				</TopbarButton>
			</PopoverTrigger>
			<PopoverContent
				horizontal="right"
				css={{ ".MuiPaper-root": { width: 300 } }}
			>
				<div
					css={{
						color: theme.palette.text.secondary,
						padding: 20,
						borderBottom: `1px solid ${theme.palette.divider}`,
					}}
				>
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
