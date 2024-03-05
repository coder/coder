import AddIcon from "@mui/icons-material/AddOutlined";
import Inventory2 from "@mui/icons-material/Inventory2";
import NoteAddOutlined from "@mui/icons-material/NoteAddOutlined";
import UploadOutlined from "@mui/icons-material/UploadOutlined";
import Button from "@mui/material/Button";
import type { FC } from "react";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
} from "components/MoreMenu/MoreMenu";

type CreateTemplateButtonProps = {
  onNavigate: (path: string) => void;
};

export const CreateTemplateButton: FC<CreateTemplateButtonProps> = ({
  onNavigate,
}) => {
  return (
    <MoreMenu>
      <MoreMenuTrigger>
        <Button startIcon={<AddIcon />} variant="contained">
          Create Template
        </Button>
      </MoreMenuTrigger>
      <MoreMenuContent>
        <MoreMenuItem
          onClick={() => {
            onNavigate(`/templates/new?exampleId=scratch`);
          }}
        >
          <NoteAddOutlined />
          From scratch
        </MoreMenuItem>
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
