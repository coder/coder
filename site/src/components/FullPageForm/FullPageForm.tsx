import { Margins } from "components/Margins/Margins";
import { type FC, type ReactNode } from "react";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";

export interface FullPageFormProps {
  title: string;
  detail?: ReactNode;
}

export const FullPageForm: FC<React.PropsWithChildren<FullPageFormProps>> = ({
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
