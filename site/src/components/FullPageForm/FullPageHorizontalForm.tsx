import { Margins } from "components/Margins/Margins";
import { FC, ReactNode } from "react";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";
import Button from "@mui/material/Button";

export interface FullPageHorizontalFormProps {
  title: string;
  detail?: ReactNode;
  onCancel?: () => void;
}

export const FullPageHorizontalForm: FC<
  React.PropsWithChildren<FullPageHorizontalFormProps>
> = ({ title, detail, onCancel, children }) => {
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
