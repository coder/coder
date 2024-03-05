import type { FC, ReactNode } from "react";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";

export interface FullPageFormProps {
  title: string;
  detail?: ReactNode;
  children?: ReactNode;
}

export const FullPageForm: FC<FullPageFormProps> = ({
  title,
  detail,
  children,
}) => {
  return (
    <Margins size="small">
      <PageHeader css={{ paddingBottom: 24 }}>
        <PageHeaderTitle>{title}</PageHeaderTitle>
        {detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
      </PageHeader>

      <main>{children}</main>
    </Margins>
  );
};
