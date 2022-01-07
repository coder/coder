import React from "react"

import { makeStyles } from "@material-ui/core"

export const Triangle: React.FC = () => {
  const size = 100

  const styles = useStyles()

  const transparent = `${size}px solid transparent`
  const colored = `${size / 0.86}px solid black`

  return (
    <div
      className={styles.triangle}
      style={{
        width: 0,
        height: 0,
        borderLeft: transparent,
        borderRight: transparent,
        borderBottom: colored,
      }}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  "@keyframes spin": {
    from: {
      transform: "rotateY(0deg)",
    },
    to: {
      transform: "rotateY(180deg)",
    },
  },
  triangle: {
    animation: "$spin 1s ease-in-out infinite alternate both",
  },
}))
