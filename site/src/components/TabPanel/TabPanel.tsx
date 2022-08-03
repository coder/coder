import { makeStyles } from "@material-ui/core/styles"
import { fade } from "@material-ui/core/styles/colorManipulator"
import { FC } from "react"
import { TabSidebar, TabSidebarItem } from "../TabSidebar/TabSidebar"

export type AdminMenuItemCallback = (menuItem: string) => void

export interface TabPanelProps {
  title: string
  menuItems: TabSidebarItem[]
}

export const TabPanel: FC<React.PropsWithChildren<TabPanelProps>> = ({
  children,
  title,
  menuItems,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.inner}>
        <div className={styles.menuPanel}>
          <div className={styles.title}>{title}</div>
          <TabSidebar menuItems={menuItems} />
        </div>

        <div className={styles.contentPanel}>{children}</div>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    minHeight: 400,
    marginBottom: theme.spacing(2),
  },

  inner: {
    display: "flex",
    maxWidth: 1920,
    padding: theme.spacing(5, 3.5, 0, 4),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      padding: 0,
    },
  },

  icon: {
    fontSize: 100,
    position: "absolute",
    left: -50,
    top: 31,
    color: fade(theme.palette.common.black, 0.1),
    transition: "transform 0.3s ease",
    zIndex: -1,
  },

  menuPanel: {
    paddingRight: 40,

    [theme.breakpoints.down("sm")]: {
      padding: 0,
    },
  },

  title: {
    marginTop: theme.spacing(6),
    fontSize: 32,
  },

  contentPanel: {
    display: "flex",
    flexDirection: "column",
    width: "100%",
    maxWidth: 930,
  },

  [theme.breakpoints.up("lg")]: {
    icon: {
      position: "relative",
      top: -11,
      left: 30,
    },
    contentPanel: {
      width: 930,
    },
  },

  [theme.breakpoints.down("lg")]: {
    contentPanel: {
      width: 890,
    },
  },
  [theme.breakpoints.down("sm")]: {
    contentPanel: {
      width: "auto",
    },
  },
}))
