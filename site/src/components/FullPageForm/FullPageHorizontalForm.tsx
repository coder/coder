import Button from "@mui/material/Button";
import { type FC, type ReactNode } from "react";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";

export interface FullPageHorizontalFormProps {
  title: string;
  detail?: ReactNode;
  onCancel?: () => void;
  children?: ReactNode;
}

export const FullPageHorizontalForm: FC<FullPageHorizontalFormProps> = ({
  title,
  detail,
  onCancel,
  children,
}) => {
  return (
    <Margins size="medium">
      <PageHeader
        actions={
          onCancel && (
            <Button size="small" onClick={onCancel}>
              Cancel
            </Button>
          )
        }
      >
        <PageHeaderTitle>{title}</PageHeaderTitle>
        {detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
      </PageHeader>

      <main>{children}</main>
    </Margins>
  );
};
