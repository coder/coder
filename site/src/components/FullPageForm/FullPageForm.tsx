import { Margins } from "components/Margins/Margins";
import { FC, ReactNode } from "react";
import {
  PageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/PageHeader";
import { makeStyles } from "@mui/styles";

export interface FullPageFormProps {
  title: string;
  detail?: ReactNode;
}

export const FullPageForm: FC<React.PropsWithChildren<FullPageFormProps>> = ({
  title,
  detail,
  children,
}) => {
  const styles = useStyles();

  return (
    <Margins size="small">
      <PageHeader className={styles.pageHeader}>
        <PageHeaderTitle>{title}</PageHeaderTitle>
        {detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
      </PageHeader>

      <main>{children}</main>
    </Margins>
  );
};

const useStyles = makeStyles((theme) => ({
  pageHeader: {
    paddingBottom: theme.spacing(3),
  },
}));
