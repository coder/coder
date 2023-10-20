import { Margins } from "components/Margins/Margins";
import { type FC, type ReactNode } from "react";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";
import { useTheme } from "@emotion/react";

export interface FullPageFormProps {
  title: string;
  detail?: ReactNode;
}

export const FullPageForm: FC<React.PropsWithChildren<FullPageFormProps>> = ({
  title,
  detail,
  children,
}) => {
  const theme = useTheme();

  return (
    <Margins size="small">
      <PageHeader css={{ paddingBottom: theme.spacing(3) }}>
        <PageHeaderTitle>{title}</PageHeaderTitle>
        {detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
      </PageHeader>

      <main>{children}</main>
    </Margins>
  );
};
