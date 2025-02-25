import Inventory2 from "@mui/icons-material/Inventory2";
import NoteAddOutlined from "@mui/icons-material/NoteAddOutlined";
import UploadOutlined from "@mui/icons-material/UploadOutlined";
import { Button } from "components/Button/Button";
import {
	MoreMenu,
	MoreMenuContent,
	MoreMenuItem,
	MoreMenuTrigger,
} from "components/MoreMenu/MoreMenu";
import { PlusIcon } from "lucide-react";
import type { FC } from "react";

type CreateTemplateButtonProps = {
	onNavigate: (path: string) => void;
};

export const CreateTemplateButton: FC<CreateTemplateButtonProps> = ({
	onNavigate,
}) => {
	return (
		<MoreMenu>
			<MoreMenuTrigger>
				<Button>
					<PlusIcon />
					Create template
				</Button>
			</MoreMenuTrigger>
			<MoreMenuContent>
				<MoreMenuItem
					onClick={() => {
						onNavigate("/templates/new");
					}}
				>
					<UploadOutlined />
					Upload template
				</MoreMenuItem>
				<MoreMenuItem
					onClick={() => {
						onNavigate("/starter-templates");
					}}
				>
					<Inventory2 />
					Choose a starter template
				</MoreMenuItem>
			</MoreMenuContent>
		</MoreMenu>
	);
};
